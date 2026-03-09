package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"

	"despatch/internal/mail"
	mailsecclient "despatch/internal/mailsec"
	"despatch/internal/middleware"
	"despatch/internal/models"
	"despatch/internal/service"
	"despatch/internal/store"
	"despatch/internal/util"
)

func (h *Handlers) V2Login(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSetupComplete(w, r) {
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	token, user, err := h.svc.Login(r.Context(), req.Email, req.Password, r.RemoteAddr, r.UserAgent())
	if err != nil {
		if errors.Is(err, service.ErrPAMVerifierDown) {
			util.WriteError(w, http.StatusBadGateway, "pam_verifier_unavailable", "cannot validate PAM credentials", middleware.RequestID(r.Context()))
			return
		}
		status := 401
		code := "invalid_credentials"
		if err == service.ErrPendingApproval {
			status, code = 403, "pending_approval"
		} else if err == service.ErrSuspended {
			status, code = 403, "suspended"
		}
		util.WriteError(w, status, code, err.Error(), middleware.RequestID(r.Context()))
		return
	}
	csrfToken, err := randomToken()
	if err != nil {
		util.WriteError(w, 500, "internal_error", "failed to generate token", middleware.RequestID(r.Context()))
		return
	}
	sess, stage, err := h.resolveLoginStage(r.Context(), w, r, token, user)
	if err != nil {
		util.WriteError(w, 500, "internal_error", "cannot finalize login session", middleware.RequestID(r.Context()))
		return
	}
	h.setAuthCookies(w, r, token, csrfToken)
	out := map[string]any{
		"user_id":              user.ID,
		"email":                user.Email,
		"role":                 user.Role,
		"csrf_token":           csrfToken,
		"session_active":       true,
		"mail_secret_required": strings.TrimSpace(sess.MailSecret) == "",
	}
	applyAuthStageFields(out, stage)
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V1PasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	h.passkeyLoginBegin(w, r)
}

func (h *Handlers) V2PasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	h.passkeyLoginBegin(w, r)
}

func (h *Handlers) passkeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSetupComplete(w, r) {
		return
	}
	caps := h.authCapabilities(r)
	if available, _ := caps["passkey_passwordless_available"].(bool); !available {
		reason := strings.TrimSpace(fmt.Sprintf("%v", caps["reason"]))
		if reason == "mailsec_unavailable" {
			util.WriteError(w, http.StatusServiceUnavailable, "mailsec_unavailable", "mailsec service is required for passkey login", middleware.RequestID(r.Context()))
			return
		}
		if reason == "insecure_origin" {
			util.WriteError(w, http.StatusBadRequest, "webauthn_insecure_origin", "passkey login requires HTTPS or localhost", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusServiceUnavailable, "passkey_unavailable", "passkey login is not available", middleware.RequestID(r.Context()))
		return
	}
	webAuthnContext, ok := h.requireWebAuthnContext(w, r)
	if !ok {
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, true); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	emailProvided := strings.TrimSpace(req.Email) != ""

	_ = h.svc.Store().DeleteSettingsByPrefixOlderThan(r.Context(), passkeyLoginChallengeSettingsPrefix(), time.Now().UTC().Add(-30*time.Minute))

	challengeID := uuid.NewString()
	cookieNonce, err := randomToken()
	if err != nil {
		util.WriteError(w, 500, "webauthn_begin_failed", "cannot generate challenge", middleware.RequestID(r.Context()))
		return
	}
	challenge, err := randomToken()
	if err != nil {
		util.WriteError(w, 500, "webauthn_begin_failed", "cannot generate challenge", middleware.RequestID(r.Context()))
		return
	}
	state := passkeyLoginChallengeState{
		Challenge:       challenge,
		RPID:            webAuthnContext.RPID,
		Origins:         webAuthnContext.Origins,
		ExpiresAt:       time.Now().UTC().Add(5 * time.Minute),
		AttemptCount:    0,
		MaxAttempts:     5,
		CookieTokenHash: trustedDeviceTokenHash(cookieNonce),
	}
	if err := h.storePasskeyLoginChallenge(r.Context(), challengeID, state); err != nil {
		util.WriteError(w, 500, "webauthn_begin_failed", "cannot persist challenge", middleware.RequestID(r.Context()))
		return
	}
	h.setPasskeyLoginChallengeCookie(w, r, challengeID, cookieNonce, state.ExpiresAt)

	mode := "usernameless"
	beginMeta, _ := json.Marshal(map[string]any{
		"mode":           mode,
		"email_provided": emailProvided,
	})
	_ = h.svc.Store().InsertAudit(
		r.Context(),
		"anonymous",
		"auth.passkey.login.begin",
		"passkey",
		string(beginMeta),
	)
	util.WriteJSON(w, 200, map[string]any{
		"status":       "challenge_created",
		"mode":         mode,
		"challenge_id": challengeID,
		"challenge":    challenge,
		"timeout_ms":   300000,
		"rp_id":        webAuthnContext.RPID,
		"public_key": map[string]any{
			"challenge":        challenge,
			"rpId":             webAuthnContext.RPID,
			"timeout":          300000,
			"userVerification": "required",
			"allowCredentials": []map[string]any{},
		},
		"allow_credentials": []map[string]any{},
	})
}

func (h *Handlers) V1PasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	h.passkeyLoginFinish(w, r)
}

func (h *Handlers) V2PasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	h.passkeyLoginFinish(w, r)
}

func (h *Handlers) passkeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSetupComplete(w, r) {
		return
	}
	caps := h.authCapabilities(r)
	if available, _ := caps["passkey_passwordless_available"].(bool); !available {
		util.WriteError(w, http.StatusServiceUnavailable, "passkey_unavailable", "passkey login is not available", middleware.RequestID(r.Context()))
		return
	}
	webAuthnContext, ok := h.requireWebAuthnContext(w, r)
	if !ok {
		return
	}

	challengeID, cookieNonce, ok := h.readPasskeyLoginChallengeCookie(r)
	if !ok {
		util.WriteError(w, http.StatusUnauthorized, "webauthn_challenge_invalid", "challenge is missing or expired", middleware.RequestID(r.Context()))
		return
	}
	state, ok, err := h.loadPasskeyLoginChallenge(r.Context(), challengeID)
	if err != nil {
		util.WriteError(w, 500, "webauthn_finish_failed", "cannot read challenge state", middleware.RequestID(r.Context()))
		return
	}
	if !ok || time.Now().UTC().After(state.ExpiresAt) {
		h.clearPasskeyLoginChallengeCookie(w, r)
		_ = h.deletePasskeyLoginChallenge(r.Context(), challengeID)
		util.WriteError(w, http.StatusUnauthorized, "webauthn_challenge_invalid", "challenge is missing or expired", middleware.RequestID(r.Context()))
		return
	}
	if subtleConstantCompare(state.CookieTokenHash, trustedDeviceTokenHash(cookieNonce)) != 1 {
		h.clearPasskeyLoginChallengeCookie(w, r)
		_ = h.deletePasskeyLoginChallenge(r.Context(), challengeID)
		util.WriteError(w, http.StatusUnauthorized, "webauthn_challenge_invalid", "challenge is missing or expired", middleware.RequestID(r.Context()))
		return
	}
	state.AttemptCount++
	if state.MaxAttempts <= 0 {
		state.MaxAttempts = 5
	}
	if state.AttemptCount > state.MaxAttempts {
		h.clearPasskeyLoginChallengeCookie(w, r)
		_ = h.deletePasskeyLoginChallenge(r.Context(), challengeID)
		util.WriteError(w, http.StatusTooManyRequests, "rate_limited", "too many requests", middleware.RequestID(r.Context()))
		return
	}
	if err := h.storePasskeyLoginChallenge(r.Context(), challengeID, state); err != nil {
		util.WriteError(w, 500, "webauthn_finish_failed", "cannot persist challenge state", middleware.RequestID(r.Context()))
		return
	}

	var req map[string]any
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if payloadChallengeID := strings.TrimSpace(getStringValue(req, "challenge_id")); payloadChallengeID != "" && subtleConstantCompare(payloadChallengeID, challengeID) != 1 {
		util.WriteError(w, http.StatusUnauthorized, "webauthn_challenge_invalid", "challenge mismatch", middleware.RequestID(r.Context()))
		return
	}
	if payloadChallenge := strings.TrimSpace(getStringValue(req, "challenge")); payloadChallenge != "" && subtleConstantCompare(payloadChallenge, state.Challenge) != 1 {
		util.WriteError(w, http.StatusUnauthorized, "webauthn_challenge_invalid", "challenge mismatch", middleware.RequestID(r.Context()))
		return
	}

	resp := getMapValue(req, "response")
	credID := getStringValue(req, "credential_id", "raw_id", "rawId", "id")
	clientDataJSON := getStringValue(resp, "client_data_json", "clientDataJSON")
	authenticatorData := getStringValue(resp, "authenticator_data", "authenticatorData")
	signature := getStringValue(resp, "signature")
	if credID == "" || clientDataJSON == "" || authenticatorData == "" || signature == "" {
		util.WriteError(w, 400, "bad_request", "credential and response fields are required", middleware.RequestID(r.Context()))
		return
	}

	cred, err := h.findMFAWebAuthnCredentialAnyUserByAnyID(r.Context(), credID)
	if err != nil {
		util.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "invalid credentials", middleware.RequestID(r.Context()))
		return
	}
	if strings.TrimSpace(state.UserID) != "" && subtleConstantCompare(strings.TrimSpace(state.UserID), strings.TrimSpace(cred.UserID)) != 1 {
		util.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "invalid credentials", middleware.RequestID(r.Context()))
		return
	}
	if len(state.AllowCredentialsJSON) > 0 {
		matched := false
		for _, item := range state.AllowCredentialsJSON {
			if webAuthnCredentialIDEqual(item, credID) || webAuthnCredentialIDEqual(item, cred.CredentialID) {
				matched = true
				break
			}
		}
		if !matched {
			util.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "invalid credentials", middleware.RequestID(r.Context()))
			return
		}
	}

	user, err := h.svc.Store().GetUserByID(r.Context(), cred.UserID)
	if err != nil {
		util.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "invalid credentials", middleware.RequestID(r.Context()))
		return
	}
	assertionCredentialID := canonicalWebAuthnCredentialID(credID)
	if assertionCredentialID == "" {
		assertionCredentialID = strings.TrimSpace(credID)
	}
	mailsecResult, err := h.callMailSecOperation(r.Context(), "webauthn.assertion.finish", user.ID, map[string]any{
		"challenge":                     state.Challenge,
		"rp_id":                         firstNonEmpty(state.RPID, webAuthnContext.RPID),
		"origins":                       firstNonEmptySlice(state.Origins, webAuthnContext.Origins),
		"credential_id":                 assertionCredentialID,
		"client_data_json_b64url":       clientDataJSON,
		"authenticator_data_b64url":     authenticatorData,
		"signature_b64url":              signature,
		"stored_public_key_cose_b64url": cred.PublicKey,
		"stored_sign_count":             cred.SignCount,
		"require_user_verification":     true,
	})
	if err != nil {
		util.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "invalid credentials", middleware.RequestID(r.Context()))
		return
	}
	nextSignCount, signCountOK := getInt64Value(mailsecResult, "sign_count")
	if !signCountOK || nextSignCount < 0 {
		util.WriteError(w, 500, "webauthn_finish_failed", "mailsec response missing sign_count", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().TouchMFAWebAuthnCredential(r.Context(), cred.UserID, cred.ID, nextSignCount); err != nil {
		util.WriteError(w, 500, "webauthn_finish_failed", "cannot update credential state", middleware.RequestID(r.Context()))
		return
	}

	mailSecret := ""
	if storedSecret, hasSecret, err := h.svc.Store().GetUserMailSecret(r.Context(), user.ID); err == nil && hasSecret {
		mailSecret = storedSecret
	}
	sessionToken, _, err := h.svc.CreatePasskeySession(
		r.Context(),
		user,
		middleware.ClientIP(r, h.cfg.TrustProxy),
		r.UserAgent(),
		mailSecret,
	)
	if err != nil {
		util.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "invalid credentials", middleware.RequestID(r.Context()))
		return
	}
	csrfToken, err := randomToken()
	if err != nil {
		util.WriteError(w, 500, "internal_error", "failed to generate token", middleware.RequestID(r.Context()))
		return
	}
	sess, stage, err := h.resolveLoginStage(r.Context(), w, r, sessionToken, user)
	if err != nil {
		util.WriteError(w, 500, "internal_error", "cannot finalize login session", middleware.RequestID(r.Context()))
		return
	}
	h.setAuthCookies(w, r, sessionToken, csrfToken)
	h.clearPasskeyLoginChallengeCookie(w, r)
	_ = h.deletePasskeyLoginChallenge(r.Context(), challengeID)

	meta, _ := json.Marshal(map[string]any{
		"auth_method": "passkey",
		"credential":  cred.ID,
	})
	_ = h.svc.Store().InsertAudit(r.Context(), user.ID, "auth.passkey.login.finish", user.ID, string(meta))

	out := map[string]any{
		"user_id":              user.ID,
		"email":                user.Email,
		"role":                 user.Role,
		"csrf_token":           csrfToken,
		"session_active":       true,
		"mail_secret_required": strings.TrimSpace(sess.MailSecret) == "",
	}
	applyAuthStageFields(out, stage)
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V1SessionMailSecretUnlock(w http.ResponseWriter, r *http.Request) {
	h.sessionMailSecretUnlock(w, r)
}

func (h *Handlers) V2SessionMailSecretUnlock(w http.ResponseWriter, r *http.Request) {
	h.sessionMailSecretUnlock(w, r)
}

func (h *Handlers) sessionMailSecretUnlock(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.User(r.Context())
	if !ok {
		util.WriteError(w, http.StatusUnauthorized, "session_missing", "authentication required", middleware.RequestID(r.Context()))
		return
	}
	sess, ok := middleware.Session(r.Context())
	if !ok {
		util.WriteError(w, http.StatusUnauthorized, "session_missing", "authentication required", middleware.RequestID(r.Context()))
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if err := h.svc.UnlockSessionMailSecret(r.Context(), u.ID, sess.ID, req.Password); err != nil {
		if errors.Is(err, service.ErrPAMVerifierDown) {
			util.WriteError(w, http.StatusBadGateway, "pam_verifier_unavailable", "cannot validate PAM credentials", middleware.RequestID(r.Context()))
			return
		}
		if errors.Is(err, service.ErrInvalidCredentials) {
			util.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "invalid credentials", middleware.RequestID(r.Context()))
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, http.StatusUnauthorized, "session_invalid", "invalid session", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "mail_secret_unlock_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{
		"status":               "ok",
		"mail_secret_required": false,
	})
}

func (h *Handlers) V2MFATOTPVerify(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req struct {
		Code           string `json:"code"`
		RememberDevice bool   `json:"remember_device"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	rec, err := h.svc.Store().GetMFATOTP(r.Context(), u.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "totp_not_enrolled", "totp not enrolled", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if !rec.Enabled {
		util.WriteError(w, 400, "totp_not_enabled", "totp is not enabled", middleware.RequestID(r.Context()))
		return
	}
	secret, err := util.DecryptString(util.Derive32ByteKey(h.cfg.SessionEncryptKey), rec.SecretEnc)
	if err != nil {
		util.WriteError(w, 500, "internal_error", "cannot read totp secret", middleware.RequestID(r.Context()))
		return
	}
	valid, err := h.verifyTOTPCode(r.Context(), u.ID, secret, req.Code)
	if err != nil {
		util.WriteError(w, http.StatusServiceUnavailable, "totp_verifier_unavailable", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if !valid {
		util.WriteError(w, 401, "invalid_totp_code", "invalid code", middleware.RequestID(r.Context()))
		return
	}
	sess, ok := middleware.Session(r.Context())
	if !ok {
		util.WriteError(w, http.StatusUnauthorized, "session_missing", "authentication required", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().SetSessionMFAVerified(r.Context(), sess.ID, "totp"); err != nil {
		util.WriteError(w, 500, "internal_error", "cannot mark session mfa state", middleware.RequestID(r.Context()))
		return
	}
	if req.RememberDevice {
		if err := h.issueTrustedDevice(r.Context(), w, r, u.ID); err != nil {
			util.WriteError(w, 500, "internal_error", "cannot remember trusted device", middleware.RequestID(r.Context()))
			return
		}
	}
	util.WriteJSON(w, 200, map[string]any{"status": "ok", "verified": true, "auth_stage": "authenticated"})
}

func (h *Handlers) V2MFAWebAuthnBegin(w http.ResponseWriter, r *http.Request) {
	if !h.mailSecRuntimeEnabled() {
		util.WriteError(w, http.StatusServiceUnavailable, "mailsec_unavailable", "mailsec service is required for webauthn", middleware.RequestID(r.Context()))
		return
	}
	u, _ := middleware.User(r.Context())
	sess, ok := middleware.Session(r.Context())
	if !ok {
		util.WriteError(w, http.StatusUnauthorized, "session_missing", "authentication required", middleware.RequestID(r.Context()))
		return
	}
	creds, err := h.svc.Store().ListMFAWebAuthnCredentials(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "webauthn_begin_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if len(creds) == 0 {
		util.WriteError(w, 404, "webauthn_not_enrolled", "no webauthn credentials enrolled", middleware.RequestID(r.Context()))
		return
	}
	webAuthnContext, ok := h.requireWebAuthnContext(w, r)
	if !ok {
		return
	}
	rpID, origins := webAuthnContext.RPID, webAuthnContext.Origins
	challenge, err := randomToken()
	if err != nil {
		util.WriteError(w, 500, "webauthn_begin_failed", "cannot generate challenge", middleware.RequestID(r.Context()))
		return
	}
	state := webAuthnChallengeState{
		UserID:    u.ID,
		Mode:      "login",
		Challenge: challenge,
		ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
		RPID:      rpID,
		Origins:   origins,
	}
	if err := h.storeWebAuthnChallenge(r.Context(), webAuthnLoginChallengeSettingKey(sess.ID), state); err != nil {
		util.WriteError(w, 500, "webauthn_begin_failed", "cannot persist challenge", middleware.RequestID(r.Context()))
		return
	}
	allowCredentials := make([]map[string]any, 0, len(creds))
	for _, cred := range creds {
		credentialID := canonicalWebAuthnCredentialID(cred.CredentialID)
		if credentialID == "" {
			credentialID = strings.TrimSpace(cred.CredentialID)
		}
		allowCredentials = append(allowCredentials, map[string]any{
			"id":         credentialID,
			"type":       "public-key",
			"name":       cred.Name,
			"transports": parseWebAuthnTransportsJSON(cred.TransportsJSON),
		})
	}
	util.WriteJSON(w, 200, map[string]any{
		"status":     "challenge_created",
		"challenge":  challenge,
		"timeout_ms": 300000,
		"rp_id":      rpID,
		"public_key": map[string]any{
			"challenge":        challenge,
			"rpId":             rpID,
			"timeout":          300000,
			"userVerification": "preferred",
			"allowCredentials": allowCredentials,
		},
		"allow_credentials": allowCredentials,
	})
}

func (h *Handlers) V2MFAWebAuthnFinish(w http.ResponseWriter, r *http.Request) {
	if !h.mailSecRuntimeEnabled() {
		util.WriteError(w, http.StatusServiceUnavailable, "mailsec_unavailable", "mailsec service is required for webauthn", middleware.RequestID(r.Context()))
		return
	}
	webAuthnContext, ok := h.requireWebAuthnContext(w, r)
	if !ok {
		return
	}
	u, _ := middleware.User(r.Context())
	sess, ok := middleware.Session(r.Context())
	if !ok {
		util.WriteError(w, http.StatusUnauthorized, "session_missing", "authentication required", middleware.RequestID(r.Context()))
		return
	}
	var req map[string]any
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	rememberDevice, _ := getBoolValue(req, "remember_device")
	resp := getMapValue(req, "response")
	credID := getStringValue(req, "credential_id", "raw_id", "rawId", "id")
	clientDataJSON := getStringValue(resp, "client_data_json", "clientDataJSON")
	authenticatorData := getStringValue(resp, "authenticator_data", "authenticatorData")
	signature := getStringValue(resp, "signature")
	if credID == "" {
		util.WriteError(w, 400, "bad_request", "credential id is required", middleware.RequestID(r.Context()))
		return
	}
	if clientDataJSON == "" || authenticatorData == "" || signature == "" {
		util.WriteError(w, 400, "bad_request", "response.clientDataJSON, response.authenticatorData and response.signature are required", middleware.RequestID(r.Context()))
		return
	}
	state, ok, err := h.loadWebAuthnChallenge(r.Context(), webAuthnLoginChallengeSettingKey(sess.ID))
	if err != nil {
		util.WriteError(w, 500, "webauthn_finish_failed", "cannot read challenge state", middleware.RequestID(r.Context()))
		return
	}
	if !ok || state.Mode != "login" || state.UserID != u.ID || time.Now().UTC().After(state.ExpiresAt) {
		util.WriteError(w, 401, "webauthn_challenge_invalid", "challenge is missing or expired", middleware.RequestID(r.Context()))
		return
	}
	cred, err := h.findMFAWebAuthnCredentialByAnyID(r.Context(), u.ID, credID)
	if err != nil {
		util.WriteError(w, 401, "webauthn_credential_invalid", "credential is not registered", middleware.RequestID(r.Context()))
		return
	}
	assertionCredentialID := canonicalWebAuthnCredentialID(credID)
	if assertionCredentialID == "" {
		assertionCredentialID = strings.TrimSpace(credID)
	}
	mailsecResult, err := h.callMailSecOperation(r.Context(), "webauthn.assertion.finish", u.ID, map[string]any{
		"challenge":                     state.Challenge,
		"rp_id":                         firstNonEmpty(state.RPID, webAuthnContext.RPID),
		"origins":                       firstNonEmptySlice(state.Origins, webAuthnContext.Origins),
		"credential_id":                 assertionCredentialID,
		"client_data_json_b64url":       clientDataJSON,
		"authenticator_data_b64url":     authenticatorData,
		"signature_b64url":              signature,
		"stored_public_key_cose_b64url": cred.PublicKey,
		"stored_sign_count":             cred.SignCount,
		"require_user_verification":     false,
	})
	if err != nil {
		util.WriteError(w, http.StatusUnauthorized, "webauthn_verification_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	nextSignCount, ok := getInt64Value(mailsecResult, "sign_count")
	if !ok || nextSignCount < 0 {
		util.WriteError(w, 500, "webauthn_finish_failed", "mailsec response missing sign_count", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().TouchMFAWebAuthnCredential(r.Context(), u.ID, cred.ID, nextSignCount); err != nil {
		util.WriteError(w, 500, "webauthn_finish_failed", "cannot update credential state", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().SetSessionMFAVerified(r.Context(), sess.ID, "webauthn"); err != nil {
		util.WriteError(w, 500, "webauthn_finish_failed", "cannot mark session mfa state", middleware.RequestID(r.Context()))
		return
	}
	if rememberDevice {
		if err := h.issueTrustedDevice(r.Context(), w, r, u.ID); err != nil {
			util.WriteError(w, 500, "webauthn_finish_failed", "cannot remember trusted device", middleware.RequestID(r.Context()))
			return
		}
	}
	_ = h.svc.Store().DeleteSetting(r.Context(), webAuthnLoginChallengeSettingKey(sess.ID))
	util.WriteJSON(w, 200, map[string]any{"status": "ok", "verified": true, "auth_stage": "authenticated"})
}

func (h *Handlers) V2MFARecoveryCodeVerify(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	sess, ok := middleware.Session(r.Context())
	if !ok {
		util.WriteError(w, http.StatusUnauthorized, "session_missing", "authentication required", middleware.RequestID(r.Context()))
		return
	}
	var req struct {
		Code           string `json:"code"`
		RememberDevice bool   `json:"remember_device"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		util.WriteError(w, 400, "bad_request", "code is required", middleware.RequestID(r.Context()))
		return
	}
	sum := sha256.Sum256([]byte(code))
	ok, err := h.svc.Store().ConsumeRecoveryCodeHash(r.Context(), u.ID, hex.EncodeToString(sum[:]))
	if err != nil {
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if !ok {
		util.WriteError(w, 401, "invalid_recovery_code", "invalid recovery code", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().SetSessionMFAVerified(r.Context(), sess.ID, "recovery_code"); err != nil {
		util.WriteError(w, 500, "internal_error", "cannot mark session mfa state", middleware.RequestID(r.Context()))
		return
	}
	if req.RememberDevice {
		if err := h.issueTrustedDevice(r.Context(), w, r, u.ID); err != nil {
			util.WriteError(w, 500, "internal_error", "cannot remember trusted device", middleware.RequestID(r.Context()))
			return
		}
	}
	util.WriteJSON(w, 200, map[string]any{"status": "ok", "verified": true, "auth_stage": "authenticated"})
}

func (h *Handlers) V2ListAccounts(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	items, err := h.svc.Store().ListMailAccounts(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	for i := range items {
		items[i].SecretEnc = ""
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2CreateAccount(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req struct {
		DisplayName  string `json:"display_name"`
		Login        string `json:"login"`
		Password     string `json:"password"`
		IMAPHost     string `json:"imap_host"`
		IMAPPort     int    `json:"imap_port"`
		IMAPTLS      *bool  `json:"imap_tls"`
		IMAPStartTLS *bool  `json:"imap_starttls"`
		SMTPHost     string `json:"smtp_host"`
		SMTPPort     int    `json:"smtp_port"`
		SMTPTLS      *bool  `json:"smtp_tls"`
		SMTPStartTLS *bool  `json:"smtp_starttls"`
		IsDefault    bool   `json:"is_default"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	login := strings.TrimSpace(req.Login)
	if login == "" {
		util.WriteError(w, 400, "bad_request", "login is required", middleware.RequestID(r.Context()))
		return
	}
	if strings.TrimSpace(req.Password) == "" {
		util.WriteError(w, 400, "bad_request", "password is required", middleware.RequestID(r.Context()))
		return
	}
	secretEnc, err := util.EncryptString(util.Derive32ByteKey(h.cfg.SessionEncryptKey), req.Password)
	if err != nil {
		util.WriteError(w, 500, "internal_error", "cannot encrypt account secret", middleware.RequestID(r.Context()))
		return
	}
	imapTLS := true
	if req.IMAPTLS != nil {
		imapTLS = *req.IMAPTLS
	}
	imapStartTLS := false
	if req.IMAPStartTLS != nil {
		imapStartTLS = *req.IMAPStartTLS
	}
	smtpTLS := false
	if req.SMTPTLS != nil {
		smtpTLS = *req.SMTPTLS
	}
	smtpStartTLS := true
	if req.SMTPStartTLS != nil {
		smtpStartTLS = *req.SMTPStartTLS
	}
	account := models.MailAccount{
		ID:           uuid.NewString(),
		UserID:       u.ID,
		DisplayName:  strings.TrimSpace(req.DisplayName),
		Login:        login,
		SecretEnc:    secretEnc,
		IMAPHost:     firstNonEmpty(req.IMAPHost, h.cfg.IMAPHost),
		IMAPPort:     firstPositive(req.IMAPPort, h.cfg.IMAPPort),
		IMAPTLS:      imapTLS,
		IMAPStartTLS: imapStartTLS,
		SMTPHost:     firstNonEmpty(req.SMTPHost, h.cfg.SMTPHost),
		SMTPPort:     firstPositive(req.SMTPPort, h.cfg.SMTPPort),
		SMTPTLS:      smtpTLS,
		SMTPStartTLS: smtpStartTLS,
		IsDefault:    req.IsDefault,
		Status:       "active",
	}
	out, err := h.svc.Store().CreateMailAccount(r.Context(), account)
	if err != nil {
		util.WriteError(w, 400, "create_account_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	out.SecretEnc = ""
	util.WriteJSON(w, 201, out)
}

func (h *Handlers) V2UpdateAccount(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := chi.URLParam(r, "id")
	current, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req struct {
		DisplayName  *string `json:"display_name"`
		Login        *string `json:"login"`
		Password     *string `json:"password"`
		IMAPHost     *string `json:"imap_host"`
		IMAPPort     *int    `json:"imap_port"`
		IMAPTLS      *bool   `json:"imap_tls"`
		IMAPStartTLS *bool   `json:"imap_starttls"`
		SMTPHost     *string `json:"smtp_host"`
		SMTPPort     *int    `json:"smtp_port"`
		SMTPTLS      *bool   `json:"smtp_tls"`
		SMTPStartTLS *bool   `json:"smtp_starttls"`
		IsDefault    *bool   `json:"is_default"`
		Status       *string `json:"status"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if req.DisplayName != nil {
		current.DisplayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.Login != nil {
		current.Login = strings.TrimSpace(*req.Login)
	}
	if req.Password != nil && strings.TrimSpace(*req.Password) != "" {
		enc, err := util.EncryptString(util.Derive32ByteKey(h.cfg.SessionEncryptKey), strings.TrimSpace(*req.Password))
		if err != nil {
			util.WriteError(w, 500, "internal_error", "cannot encrypt account secret", middleware.RequestID(r.Context()))
			return
		}
		current.SecretEnc = enc
	}
	if req.IMAPHost != nil {
		current.IMAPHost = strings.TrimSpace(*req.IMAPHost)
	}
	if req.IMAPPort != nil && *req.IMAPPort > 0 {
		current.IMAPPort = *req.IMAPPort
	}
	if req.IMAPTLS != nil {
		current.IMAPTLS = *req.IMAPTLS
	}
	if req.IMAPStartTLS != nil {
		current.IMAPStartTLS = *req.IMAPStartTLS
	}
	if req.SMTPHost != nil {
		current.SMTPHost = strings.TrimSpace(*req.SMTPHost)
	}
	if req.SMTPPort != nil && *req.SMTPPort > 0 {
		current.SMTPPort = *req.SMTPPort
	}
	if req.SMTPTLS != nil {
		current.SMTPTLS = *req.SMTPTLS
	}
	if req.SMTPStartTLS != nil {
		current.SMTPStartTLS = *req.SMTPStartTLS
	}
	if req.IsDefault != nil {
		current.IsDefault = *req.IsDefault
	}
	if req.Status != nil {
		current.Status = strings.TrimSpace(*req.Status)
	}
	out, err := h.svc.Store().UpdateMailAccount(r.Context(), current)
	if err != nil {
		util.WriteError(w, 400, "update_account_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	out.SecretEnc = ""
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2DeleteAccount(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.svc.Store().DeleteMailAccount(r.Context(), u.ID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "delete_account_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "deleted"})
}

func (h *Handlers) V2ActivateAccount(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.svc.Store().SetActiveMailAccount(r.Context(), u.ID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "activate_account_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "ok", "active_account_id": id})
}

func (h *Handlers) V2ListIdentities(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := chi.URLParam(r, "id")
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	items, err := h.svc.Store().ListMailIdentities(r.Context(), accountID)
	if err != nil {
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2CreateIdentity(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := chi.URLParam(r, "id")
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req models.MailIdentity
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	req.ID = uuid.NewString()
	req.AccountID = accountID
	out, err := h.svc.Store().CreateMailIdentity(r.Context(), req)
	if err != nil {
		util.WriteError(w, 400, "create_identity_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 201, out)
}

func (h *Handlers) V2UpdateIdentity(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := chi.URLParam(r, "id")
	current, err := h.svc.Store().GetMailIdentityByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "identity_not_found", "identity not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, current.AccountID); err != nil {
		util.WriteError(w, 403, "forbidden", "identity does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	var req models.MailIdentity
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	req.ID = id
	req.AccountID = current.AccountID
	if strings.TrimSpace(req.ID) == "" {
		util.WriteError(w, 400, "bad_request", "id is required", middleware.RequestID(r.Context()))
		return
	}
	out, err := h.svc.Store().UpdateMailIdentity(r.Context(), req)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "identity_not_found", "identity not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 400, "update_identity_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2DeleteIdentity(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := chi.URLParam(r, "id")
	current, err := h.svc.Store().GetMailIdentityByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "identity_not_found", "identity not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, current.AccountID); err != nil {
		util.WriteError(w, 403, "forbidden", "identity does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().DeleteMailIdentity(r.Context(), current.AccountID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "identity_not_found", "identity not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "delete_identity_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "deleted"})
}

func (h *Handlers) V2ListMailboxMappings(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" {
		util.WriteError(w, 400, "bad_request", "account_id is required", middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	items, err := h.svc.Store().ListMailboxMappings(r.Context(), accountID)
	if err != nil {
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2UpsertMailboxMapping(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req models.MailboxMapping
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, req.AccountID); err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	if id := chi.URLParam(r, "id"); strings.TrimSpace(id) != "" {
		req.ID = id
	}
	if strings.TrimSpace(req.ID) == "" {
		req.ID = uuid.NewString()
	}
	if req.Priority == 0 {
		req.Priority = 100
	}
	out, err := h.svc.Store().UpsertMailboxMapping(r.Context(), req)
	if err != nil {
		util.WriteError(w, 400, "upsert_mailbox_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2DeleteMailboxMapping(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	id := chi.URLParam(r, "id")
	if accountID == "" || id == "" {
		util.WriteError(w, 400, "bad_request", "account_id and id are required", middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().DeleteMailboxMapping(r.Context(), accountID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "mailbox_mapping_not_found", "mailbox mapping not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "delete_mailbox_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "deleted"})
}

func (h *Handlers) V2ListThreads(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" {
		util.WriteError(w, 400, "bad_request", "account_id is required", middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	page, pageSize := parsePaginationV2(r)
	items, total, err := h.svc.Store().ListThreads(
		r.Context(),
		accountID,
		strings.TrimSpace(r.URL.Query().Get("mailbox")),
		strings.TrimSpace(r.URL.Query().Get("sort")),
		pageSize,
		(page-1)*pageSize,
	)
	if err != nil {
		util.WriteError(w, 500, "threads_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items, "page": page, "page_size": pageSize, "total": total})
}

func (h *Handlers) V2GetThread(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" {
		util.WriteError(w, 400, "bad_request", "account_id is required", middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	threadID := chi.URLParam(r, "id")
	items, err := h.svc.Store().ListMessagesByThread(r.Context(), accountID, threadID, 100, 0)
	if err != nil {
		util.WriteError(w, 500, "thread_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"id": threadID, "items": items})
}

func (h *Handlers) V2GetIndexedMessage(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" {
		util.WriteError(w, 400, "bad_request", "account_id is required", middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	id := chi.URLParam(r, "id")
	msg, err := h.svc.Store().GetIndexedMessageByID(r.Context(), accountID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "message_not_found", "message not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "message_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	attachments, _ := h.svc.Store().GetIndexedMessageAttachments(r.Context(), accountID, id)
	util.WriteJSON(w, 200, map[string]any{"message": msg, "attachments": attachments})
}

func (h *Handlers) V2GetIndexedMessageRaw(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" {
		util.WriteError(w, 400, "bad_request", "account_id is required", middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	id := chi.URLParam(r, "id")
	msg, err := h.svc.Store().GetIndexedMessageByID(r.Context(), accountID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "message_not_found", "message not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "message_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(msg.RawSource))
}

func (h *Handlers) V2BulkMessages(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req struct {
		AccountID string   `json:"account_id"`
		IDs       []string `json:"ids"`
		Action    string   `json:"action"`
		Mailbox   string   `json:"mailbox"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, req.AccountID); err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	applied := make([]string, 0, len(req.IDs))
	failed := make([]map[string]string, 0, len(req.IDs))

	for _, id := range req.IDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		var err error
		switch action {
		case "seen":
			err = h.svc.Store().SetIndexedMessageSeen(r.Context(), req.AccountID, id, true)
		case "unseen":
			err = h.svc.Store().SetIndexedMessageSeen(r.Context(), req.AccountID, id, false)
		case "star":
			err = h.svc.Store().SetIndexedMessageFlagged(r.Context(), req.AccountID, id, true)
		case "unstar":
			err = h.svc.Store().SetIndexedMessageFlagged(r.Context(), req.AccountID, id, false)
		case "move":
			err = h.svc.Store().MoveIndexedMessageMailbox(r.Context(), req.AccountID, id, req.Mailbox)
		case "archive":
			err = h.svc.Store().MoveIndexedMessageMailbox(r.Context(), req.AccountID, id, "Archive")
		case "spam":
			err = h.svc.Store().MoveIndexedMessageMailbox(r.Context(), req.AccountID, id, "Junk")
		case "delete":
			err = h.svc.Store().DeleteIndexedMessage(r.Context(), req.AccountID, id)
		default:
			util.WriteError(w, 400, "bad_request", "unsupported action", middleware.RequestID(r.Context()))
			return
		}
		if err == nil {
			applied = append(applied, id)
			continue
		}
		code := "action_failed"
		if errors.Is(err, store.ErrNotFound) {
			code = "message_not_found"
		}
		failed = append(failed, map[string]string{"id": id, "code": code, "message": err.Error()})
	}
	util.WriteJSON(w, 200, map[string]any{"status": "ok", "applied": applied, "failed": failed})
}

func (h *Handlers) V2AllowRemoteImages(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" {
		util.WriteError(w, 400, "bad_request", "account_id is required", middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	id := chi.URLParam(r, "id")
	msg, err := h.svc.Store().GetIndexedMessageByID(r.Context(), accountID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "message_not_found", "message not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "message_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().SetIndexedMessageRemoteImagesAllowed(r.Context(), accountID, id, true); err != nil {
		util.WriteError(w, 500, "allow_remote_images_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	_ = h.svc.Store().AddRemoteImageAllowlist(r.Context(), u.ID, id, msg.FromValue)
	util.WriteJSON(w, 200, map[string]any{"status": "ok", "allowed": true})
}

func (h *Handlers) V2DecryptIndexedMessage(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" {
		util.WriteError(w, 400, "bad_request", "account_id is required", middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	id := chi.URLParam(r, "id")
	msg, err := h.svc.Store().GetIndexedMessageByID(r.Context(), accountID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "message_not_found", "message not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "message_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req struct {
		Provider   string `json:"provider"`
		KeyringID  string `json:"keyring_id"`
		Passphrase string `json:"passphrase"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitLarge, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	keyringID := strings.TrimSpace(req.KeyringID)
	if keyringID == "" {
		util.WriteError(w, 400, "bad_request", "keyring_id is required", middleware.RequestID(r.Context()))
		return
	}
	keyring, err := h.svc.Store().GetCryptoKeyringByID(r.Context(), u.ID, keyringID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "crypto_keyring_not_found", "crypto keyring not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "crypto_keyring_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	normalizedKind := normalizeCryptoKeyringKind(keyring.Kind)
	if !strings.HasSuffix(normalizedKind, "_private") {
		util.WriteError(w, 400, "bad_request", "keyring must be a private keyring", middleware.RequestID(r.Context()))
		return
	}
	if strings.TrimSpace(keyring.PrivateKeyEnc) == "" {
		util.WriteError(w, 400, "bad_request", "keyring does not contain private key material", middleware.RequestID(r.Context()))
		return
	}
	privateKey, err := util.DecryptString(util.Derive32ByteKey(h.cfg.SessionEncryptKey), keyring.PrivateKeyEnc)
	if err != nil {
		util.WriteError(w, 500, "crypto_private_key_decrypt_failed", "cannot decrypt private key", middleware.RequestID(r.Context()))
		return
	}
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	kindProvider := inferCryptoProvider(normalizedKind)
	if provider == "" {
		provider = kindProvider
	} else if provider != kindProvider {
		util.WriteError(w, 400, "bad_request", "provider does not match keyring kind", middleware.RequestID(r.Context()))
		return
	}
	var op string
	payload := map[string]any{}
	switch provider {
	case "pgp":
		op = "crypto.pgp.decrypt"
		payload = map[string]any{
			"ciphertext_armored":  msg.RawSource,
			"private_key_armored": privateKey,
			"passphrase":          req.Passphrase,
		}
	case "smime":
		if strings.TrimSpace(keyring.PublicKey) == "" {
			util.WriteError(w, 400, "bad_request", "smime private keyring requires cert in public_key", middleware.RequestID(r.Context()))
			return
		}
		op = "crypto.smime.decrypt"
		payload = map[string]any{
			"ciphertext_smime":       msg.RawSource,
			"private_key_pem":        privateKey,
			"cert_pem":               keyring.PublicKey,
			"private_key_passphrase": req.Passphrase,
		}
	default:
		util.WriteError(w, 400, "bad_request", "provider must be pgp or smime", middleware.RequestID(r.Context()))
		return
	}
	result, err := h.callMailSecOperation(r.Context(), op, accountID, payload)
	if err != nil {
		util.WriteError(w, 422, "crypto_decrypt_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "ok", "result": result})
}

func (h *Handlers) V2VerifyIndexedMessage(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" {
		util.WriteError(w, 400, "bad_request", "account_id is required", middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	id := chi.URLParam(r, "id")
	msg, err := h.svc.Store().GetIndexedMessageByID(r.Context(), accountID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "message_not_found", "message not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "message_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req struct {
		Provider      string `json:"provider"`
		PublicKeyring string `json:"public_keyring_id"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitLarge, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	publicKeyringID := strings.TrimSpace(req.PublicKeyring)
	if publicKeyringID == "" {
		util.WriteError(w, 400, "bad_request", "public_keyring_id is required", middleware.RequestID(r.Context()))
		return
	}
	keyring, err := h.svc.Store().GetCryptoKeyringByID(r.Context(), u.ID, publicKeyringID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "crypto_keyring_not_found", "crypto keyring not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "crypto_keyring_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if strings.TrimSpace(keyring.PublicKey) == "" {
		util.WriteError(w, 400, "bad_request", "keyring does not contain public key material", middleware.RequestID(r.Context()))
		return
	}
	normalizedKind := normalizeCryptoKeyringKind(keyring.Kind)
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	kindProvider := inferCryptoProvider(normalizedKind)
	if provider == "" {
		provider = kindProvider
	} else if provider != kindProvider {
		util.WriteError(w, 400, "bad_request", "provider does not match keyring kind", middleware.RequestID(r.Context()))
		return
	}
	var op string
	payload := map[string]any{}
	switch provider {
	case "pgp":
		op = "crypto.pgp.verify"
		payload = map[string]any{
			"signed_message_armored": msg.RawSource,
			"public_key_armored":     keyring.PublicKey,
		}
	case "smime":
		op = "crypto.smime.verify"
		payload = map[string]any{
			"signed_smime":      msg.RawSource,
			"trusted_certs_pem": []string{keyring.PublicKey},
		}
	default:
		util.WriteError(w, 400, "bad_request", "provider must be pgp or smime", middleware.RequestID(r.Context()))
		return
	}
	result, err := h.callMailSecOperation(r.Context(), op, accountID, payload)
	if err != nil {
		util.WriteError(w, 422, "crypto_verify_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "ok", "result": result})
}

func (h *Handlers) V2Search(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" {
		util.WriteError(w, 400, "bad_request", "account_id is required", middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page, pageSize := parsePaginationV2(r)
	items, total, err := h.svc.Store().SearchIndexedMessages(
		r.Context(),
		accountID,
		strings.TrimSpace(r.URL.Query().Get("mailbox")),
		q,
		pageSize,
		(page-1)*pageSize,
	)
	if err != nil {
		util.WriteError(w, 500, "search_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items, "page": page, "page_size": pageSize, "total": total})
}

func (h *Handlers) V2ListSavedSearches(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	items, err := h.svc.Store().ListSavedSearches(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "saved_searches_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2CreateSavedSearch(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req models.SavedSearch
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	req.ID = uuid.NewString()
	req.UserID = u.ID
	out, err := h.svc.Store().CreateSavedSearch(r.Context(), req)
	if err != nil {
		util.WriteError(w, 400, "create_saved_search_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 201, out)
}

func (h *Handlers) V2UpdateSavedSearch(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req models.SavedSearch
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	req.ID = chi.URLParam(r, "id")
	req.UserID = u.ID
	out, err := h.svc.Store().UpdateSavedSearch(r.Context(), req)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "saved_search_not_found", "saved search not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 400, "update_saved_search_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2DeleteSavedSearch(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.svc.Store().DeleteSavedSearch(r.Context(), u.ID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "saved_search_not_found", "saved search not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "delete_saved_search_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "deleted"})
}

func (h *Handlers) V2ListDrafts(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	page, pageSize := parsePaginationV2(r)
	items, total, err := h.svc.Store().ListDrafts(
		r.Context(),
		u.ID,
		strings.TrimSpace(r.URL.Query().Get("account_id")),
		pageSize,
		(page-1)*pageSize,
	)
	if err != nil {
		util.WriteError(w, 500, "drafts_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items, "page": page, "page_size": pageSize, "total": total})
}

func (h *Handlers) V2GetDraft(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	item, err := h.svc.Store().GetDraftByID(r.Context(), u.ID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "draft_not_found", "draft not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "draft_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, item)
}

func (h *Handlers) V2GetDraftAttachment(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	draftID := strings.TrimSpace(chi.URLParam(r, "id"))
	attachmentID := strings.TrimSpace(chi.URLParam(r, "attachment_id"))
	if _, err := h.svc.Store().GetDraftByID(r.Context(), u.ID, draftID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "draft_not_found", "draft not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "draft_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	item, err := h.svc.Store().GetDraftAttachmentByID(r.Context(), u.ID, draftID, attachmentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "draft_attachment_not_found", "draft attachment not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "draft_attachment_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	w.Header().Set("Content-Type", item.ContentType)
	disposition := "attachment"
	if item.InlinePart {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", disposition+`; filename="`+item.Filename+`"`)
	if item.SizeBytes > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(item.SizeBytes, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(item.Data)
}

func (h *Handlers) V2CreateDraft(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req models.Draft
	if err := decodeJSON(w, r, &req, jsonLimitLarge, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	req.ID = uuid.NewString()
	req.UserID = u.ID
	if strings.TrimSpace(req.AccountID) != "" {
		if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, req.AccountID); err != nil {
			util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
			return
		}
	}
	out, err := h.svc.Store().CreateDraft(r.Context(), req)
	if err != nil {
		util.WriteError(w, 400, "create_draft_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 201, out)
}

func (h *Handlers) V2UpdateDraft(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := chi.URLParam(r, "id")
	current, err := h.svc.Store().GetDraftByID(r.Context(), u.ID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "draft_not_found", "draft not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "draft_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req map[string]any
	if err := decodeJSON(w, r, &req, jsonLimitLarge, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	mergeDraftPatch(&current, req)
	current.UserID = u.ID
	if strings.TrimSpace(current.AccountID) != "" {
		if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, current.AccountID); err != nil {
			util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
			return
		}
	}
	out, err := h.svc.Store().UpdateDraft(r.Context(), current)
	if err != nil {
		util.WriteError(w, 400, "update_draft_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2UploadDraftAttachments(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	draftID := strings.TrimSpace(chi.URLParam(r, "id"))
	draft, err := h.svc.Store().GetDraftByID(r.Context(), u.ID, draftID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "draft_not_found", "draft not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "draft_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	items, err := decodeDraftAttachmentUpload(r)
	if err != nil {
		util.WriteError(w, 400, "bad_request", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	nextOrder, err := h.svc.Store().NextDraftAttachmentSortOrder(r.Context(), u.ID, draft.ID)
	if err != nil {
		util.WriteError(w, 500, "draft_attachment_upload_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	createdIDs := make([]string, 0, len(items))
	for i := range items {
		items[i].DraftID = draft.ID
		items[i].UserID = u.ID
		items[i].SortOrder = nextOrder + i
		created, createErr := h.svc.Store().CreateDraftAttachment(r.Context(), items[i])
		if createErr != nil {
			for _, insertedID := range createdIDs {
				_ = h.svc.Store().DeleteDraftAttachment(r.Context(), u.ID, draft.ID, insertedID)
			}
			util.WriteError(w, 500, "draft_attachment_upload_failed", createErr.Error(), middleware.RequestID(r.Context()))
			return
		}
		items[i] = created
		createdIDs = append(createdIDs, created.ID)
	}
	updatedDraft, refs, err := h.syncDraftAttachmentMetadata(r.Context(), draft)
	if err != nil {
		for _, insertedID := range createdIDs {
			_ = h.svc.Store().DeleteDraftAttachment(r.Context(), u.ID, draft.ID, insertedID)
		}
		util.WriteError(w, 500, "draft_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 201, map[string]any{
		"draft":    updatedDraft,
		"items":    refs,
		"uploaded": draftAttachmentRefs(items),
	})
}

func (h *Handlers) V2DeleteDraftAttachment(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	draftID := strings.TrimSpace(chi.URLParam(r, "id"))
	attachmentID := strings.TrimSpace(chi.URLParam(r, "attachment_id"))
	draft, err := h.svc.Store().GetDraftByID(r.Context(), u.ID, draftID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "draft_not_found", "draft not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "draft_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().DeleteDraftAttachment(r.Context(), u.ID, draftID, attachmentID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "draft_attachment_not_found", "draft attachment not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "draft_attachment_delete_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	updatedDraft, refs, err := h.syncDraftAttachmentMetadata(r.Context(), draft)
	if err != nil {
		util.WriteError(w, 500, "draft_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{
		"draft": updatedDraft,
		"items": refs,
	})
}

func (h *Handlers) V2DeleteDraft(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		util.WriteError(w, 400, "bad_request", "id is required", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().DeleteDraft(r.Context(), u.ID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "draft_not_found", "draft not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "delete_draft_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "deleted"})
}

func (h *Handlers) V2ListDraftVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := h.svc.Store().ListDraftVersions(r.Context(), chi.URLParam(r, "id"), 20)
	if err != nil {
		util.WriteError(w, 500, "draft_versions_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": versions})
}

func (h *Handlers) V2SendDraft(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := chi.URLParam(r, "id")
	var sendReqPayload struct {
		CryptoPassphrase string         `json:"crypto_passphrase"`
		CryptoOptions    map[string]any `json:"crypto_options"`
	}
	if err := decodeJSON(w, r, &sendReqPayload, jsonLimitLarge, true); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	draft, err := h.svc.Store().GetDraftByID(r.Context(), u.ID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "draft_not_found", "draft not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "draft_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	sendMode := strings.ToLower(strings.TrimSpace(draft.SendMode))
	if sendMode == "scheduled" && !draft.ScheduledFor.IsZero() && draft.ScheduledFor.After(time.Now().UTC()) {
		if strings.TrimSpace(draft.AccountID) == "" {
			util.WriteError(w, 400, "schedule_requires_account", "scheduled send requires an account-backed draft", middleware.RequestID(r.Context()))
			return
		}
		if err := h.svc.Store().QueueScheduledSend(r.Context(), draft); err != nil {
			util.WriteError(w, 500, "schedule_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		draft.Status = "scheduled"
		draft.LastSendError = ""
		_, _ = h.svc.Store().UpdateDraft(r.Context(), draft)
		util.WriteJSON(w, 202, map[string]any{
			"status":        "scheduled",
			"scheduled_for": draft.ScheduledFor.UTC(),
			"draft_id":      draft.ID,
		})
		return
	}
	cryptoJSON := strings.TrimSpace(draft.CryptoOptions)
	if len(sendReqPayload.CryptoOptions) > 0 {
		b, err := json.Marshal(sendReqPayload.CryptoOptions)
		if err != nil {
			util.WriteError(w, 400, "bad_request", "invalid crypto_options", middleware.RequestID(r.Context()))
			return
		}
		cryptoJSON = string(b)
	}
	replyContext := strings.EqualFold(strings.TrimSpace(draft.ComposeMode), "reply") && strings.TrimSpace(draft.ContextMessageID) != ""
	mailLogin := service.MailIdentity(u)
	mailPass := ""
	if replyContext {
		mailPass, err = h.sessionMailPasswordFromContext(r.Context())
		if err != nil {
			code := "send_failed"
			status := 400
			if errors.Is(err, store.ErrNotFound) {
				code = "context_message_not_found"
				status = 404
			}
			util.WriteError(w, status, code, err.Error(), middleware.RequestID(r.Context()))
			return
		}
	}
	sendReq, sendAccountID, markAnswered, err := h.svc.BuildDraftSendRequest(r.Context(), u, mailLogin, mailPass, draft)
	if err != nil {
		code := "send_failed"
		status := 400
		if errors.Is(err, store.ErrNotFound) {
			code = "context_message_not_found"
			status = 404
		}
		util.WriteError(w, status, code, err.Error(), middleware.RequestID(r.Context()))
		return
	}
	sendReq, err = h.applyCryptoToSendRequest(r.Context(), u, sendAccountID, sendReq, cryptoJSON, sendReqPayload.CryptoPassphrase)
	if err != nil {
		util.WriteError(w, 422, "crypto_send_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	result, err := h.v2SendWithAccount(r.Context(), u, sendAccountID, sendReq)
	if err != nil {
		if errors.Is(err, mail.ErrSMTPSenderRejected) {
			util.WriteError(w, 422, "smtp_sender_rejected", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		_ = h.svc.Store().SetDraftSendState(r.Context(), u.ID, draft.ID, "failed", err.Error())
		util.WriteError(w, 502, "send_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	_ = h.svc.Store().SetDraftSendState(r.Context(), u.ID, draft.ID, "sent", "")
	if markAnswered && strings.TrimSpace(draft.ContextMessageID) != "" {
		pass, passErr := h.sessionMailPasswordFromContext(r.Context())
		if passErr == nil {
			login := service.MailIdentity(u)
			_ = h.svc.Mail().UpdateFlags(r.Context(), login, pass, draft.ContextMessageID, mail.FlagPatch{Add: []string{"\\Answered"}})
		}
	}
	util.WriteJSON(w, 200, map[string]any{
		"status":             "sent",
		"saved_copy":         result.SavedCopy,
		"saved_copy_mailbox": result.SavedCopyMailbox,
		"warning":            result.Warning,
	})
}

func (h *Handlers) V2SendMessage(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req struct {
		AccountID        string         `json:"account_id"`
		To               []string       `json:"to"`
		CC               []string       `json:"cc"`
		BCC              []string       `json:"bcc"`
		Subject          string         `json:"subject"`
		Body             string         `json:"body"`
		CryptoOptions    map[string]any `json:"crypto_options"`
		CryptoPassphrase string         `json:"crypto_passphrase"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitLarge, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	recipients := append([]string{}, req.To...)
	recipients = append(recipients, req.CC...)
	recipients = append(recipients, req.BCC...)
	if len(recipients) == 0 {
		util.WriteError(w, 400, "bad_request", "at least one recipient is required", middleware.RequestID(r.Context()))
		return
	}
	sendReq := mail.SendRequest{
		To:      req.To,
		CC:      req.CC,
		BCC:     req.BCC,
		Subject: req.Subject,
		Body:    req.Body,
	}
	cryptoJSON := ""
	if len(req.CryptoOptions) > 0 {
		b, err := json.Marshal(req.CryptoOptions)
		if err != nil {
			util.WriteError(w, 400, "bad_request", "invalid crypto_options", middleware.RequestID(r.Context()))
			return
		}
		cryptoJSON = string(b)
	}
	sendReq, err := h.applyCryptoToSendRequest(r.Context(), u, strings.TrimSpace(req.AccountID), sendReq, cryptoJSON, req.CryptoPassphrase)
	if err != nil {
		util.WriteError(w, 422, "crypto_send_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.v2SendWithAccount(r.Context(), u, strings.TrimSpace(req.AccountID), sendReq); err != nil {
		if errors.Is(err, mail.ErrSMTPSenderRejected) {
			util.WriteError(w, 422, "smtp_sender_rejected", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 502, "smtp_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "sent"})
}

func (h *Handlers) V2ListRuleScripts(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" {
		util.WriteError(w, 400, "bad_request", "account_id is required", middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	items, err := h.svc.Store().ListSieveScripts(r.Context(), accountID)
	if err != nil {
		util.WriteError(w, 500, "rules_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2GetRuleScript(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	name := chi.URLParam(r, "name")
	if accountID == "" || name == "" {
		util.WriteError(w, 400, "bad_request", "account_id and script name are required", middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	item, err := h.svc.Store().GetSieveScript(r.Context(), accountID, name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "rule_not_found", "rule script not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "rule_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, item)
}

func (h *Handlers) V2PutRuleScript(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" {
		util.WriteError(w, 400, "bad_request", "account_id is required", middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	name := chi.URLParam(r, "name")
	var req struct {
		Body string `json:"body"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if strings.TrimSpace(name) == "" || strings.TrimSpace(req.Body) == "" {
		util.WriteError(w, 400, "bad_request", "script name and body are required", middleware.RequestID(r.Context()))
		return
	}
	if err := validateSieveScript(req.Body); err != nil {
		util.WriteError(w, 400, "invalid_sieve_script", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	sum := sha256.Sum256([]byte(req.Body))
	out, err := h.svc.Store().UpsertSieveScript(r.Context(), models.SieveScript{
		ID:          uuid.NewString(),
		AccountID:   accountID,
		ScriptName:  name,
		ScriptBody:  req.Body,
		ChecksumSHA: hex.EncodeToString(sum[:]),
		Source:      "api",
	})
	if err != nil {
		util.WriteError(w, 500, "rule_put_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2ActivateRuleScript(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	name := chi.URLParam(r, "name")
	if accountID == "" || name == "" {
		util.WriteError(w, 400, "bad_request", "account_id and script name are required", middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().ActivateSieveScript(r.Context(), accountID, name); err != nil {
		util.WriteError(w, 500, "rule_activate_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "ok", "active_script": name})
}

func (h *Handlers) V2DeleteRuleScript(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	name := chi.URLParam(r, "name")
	if accountID == "" || name == "" {
		util.WriteError(w, 400, "bad_request", "account_id and script name are required", middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().DeleteSieveScript(r.Context(), accountID, name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "rule_not_found", "rule script not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "rule_delete_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "deleted"})
}

func (h *Handlers) V2ValidateRuleScript(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Body string `json:"body"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if err := validateSieveScript(req.Body); err != nil {
		util.WriteError(w, 400, "invalid_sieve_script", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "ok", "valid": true})
}

func (h *Handlers) V2GetPreferences(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	out, err := h.svc.Store().GetUserPreferences(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "preferences_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	current, _ := h.svc.Store().GetUserPreferences(r.Context(), u.ID)
	var req map[string]any
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	applyPreferencesPatch(&current, req)
	current.UserID = u.ID
	out, err := h.svc.Store().UpsertUserPreferences(r.Context(), current)
	if err != nil {
		util.WriteError(w, 500, "preferences_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2GetMFAStatus(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	sess, _ := middleware.Session(r.Context())
	status, err := h.svc.Store().GetMFAStatus(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "mfa_status_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	stage, err := h.svc.ResolveMFAStage(r.Context(), u, &sess)
	if err != nil {
		util.WriteError(w, 500, "mfa_status_failed", "cannot resolve mfa stage", middleware.RequestID(r.Context()))
		return
	}
	out := map[string]any{
		"has_totp":             status.HasTOTP,
		"totp_enabled":         status.TOTPEnabled,
		"webauthn_credentials": status.WebAuthnCount,
		"recovery_codes":       status.RecoveryCodes,
		"recovery_unused":      status.RecoveryUnused,
	}
	applyAuthStageFields(out, stage)
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2MFAEnrollTOTP(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	setNoStoreHeaders(w)
	var req struct {
		Issuer string `json:"issuer"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, true); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	issuer := strings.TrimSpace(req.Issuer)
	if issuer == "" {
		issuer = "Despatch"
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: u.Email,
		Algorithm:   otp.AlgorithmSHA1,
		Digits:      otp.DigitsSix,
		Period:      30,
		SecretSize:  20,
	})
	if err != nil {
		util.WriteError(w, 500, "totp_enroll_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	secret := key.Secret()
	enc, err := util.EncryptString(util.Derive32ByteKey(h.cfg.SessionEncryptKey), secret)
	if err != nil {
		util.WriteError(w, 500, "totp_enroll_failed", "cannot persist secret", middleware.RequestID(r.Context()))
		return
	}
	_, err = h.svc.Store().UpsertMFATOTP(r.Context(), models.MFATOTPRecord{
		UserID:      u.ID,
		SecretEnc:   enc,
		Issuer:      issuer,
		AccountName: u.Email,
		Enabled:     false,
		UpdatedAt:   time.Now().UTC(),
	})
	if err != nil {
		util.WriteError(w, 500, "totp_enroll_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	recoveryCodes, err := generateRecoveryCodes(10)
	if err != nil {
		util.WriteError(w, 500, "totp_enroll_failed", "cannot generate recovery codes", middleware.RequestID(r.Context()))
		return
	}
	hashes := make([]string, 0, len(recoveryCodes))
	for _, code := range recoveryCodes {
		sum := sha256.Sum256([]byte(code))
		hashes = append(hashes, hex.EncodeToString(sum[:]))
	}
	if err := h.svc.Store().ReplaceMFARecoveryCodes(r.Context(), u.ID, hashes); err != nil {
		util.WriteError(w, 500, "totp_enroll_failed", "cannot persist recovery codes", middleware.RequestID(r.Context()))
		return
	}
	qrPNG, err := qrcode.Encode(key.URL(), qrcode.Medium, 256)
	if err != nil {
		util.WriteError(w, 500, "totp_enroll_failed", "cannot generate setup qr code", middleware.RequestID(r.Context()))
		return
	}
	qrDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(qrPNG)

	util.WriteJSON(w, 200, map[string]any{
		"status":           "pending_confirmation",
		"issuer":           issuer,
		"account_name":     u.Email,
		"secret":           secret,
		"manual_entry_key": secret,
		"otpauth_url":      key.URL(),
		"qr_png_data_url":  qrDataURL,
		"setup_instructions": []string{
			"Open an authenticator app on your phone.",
			"Scan the QR code (or use the manual key).",
			"Enter the 6-digit code from your app to finish setup.",
			"Save your recovery codes in a safe place before enabling MFA.",
		},
		"recovery_codes": recoveryCodes,
	})
}

func (h *Handlers) V2MFAConfirmTOTP(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	sess, ok := middleware.Session(r.Context())
	if !ok {
		util.WriteError(w, http.StatusUnauthorized, "session_missing", "authentication required", middleware.RequestID(r.Context()))
		return
	}
	var req struct {
		Code             string `json:"code"`
		RecoveryCodesAck bool   `json:"recovery_codes_ack"`
		RememberDevice   bool   `json:"remember_device"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if !req.RecoveryCodesAck {
		util.WriteError(w, 400, "recovery_codes_ack_required", "you must confirm that recovery codes were saved", middleware.RequestID(r.Context()))
		return
	}
	rec, err := h.svc.Store().GetMFATOTP(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 404, "totp_not_enrolled", "totp not enrolled", middleware.RequestID(r.Context()))
		return
	}
	secret, err := util.DecryptString(util.Derive32ByteKey(h.cfg.SessionEncryptKey), rec.SecretEnc)
	if err != nil {
		util.WriteError(w, 500, "totp_confirm_failed", "cannot read totp secret", middleware.RequestID(r.Context()))
		return
	}
	valid, err := h.verifyTOTPCode(r.Context(), u.ID, secret, req.Code)
	if err != nil {
		util.WriteError(w, http.StatusServiceUnavailable, "totp_verifier_unavailable", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if !valid {
		util.WriteError(w, 401, "invalid_totp_code", "invalid code", middleware.RequestID(r.Context()))
		return
	}
	rec.Enabled = true
	rec.EnrolledAt = time.Now().UTC()
	if _, err := h.svc.Store().UpsertMFATOTP(r.Context(), rec); err != nil {
		util.WriteError(w, 500, "totp_confirm_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().SetUserMFABackupCompleted(r.Context(), u.ID, true); err != nil {
		util.WriteError(w, 500, "totp_confirm_failed", "cannot update mfa backup state", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().SetSessionMFAVerified(r.Context(), sess.ID, "totp_setup"); err != nil {
		util.WriteError(w, 500, "totp_confirm_failed", "cannot mark session mfa state", middleware.RequestID(r.Context()))
		return
	}
	if req.RememberDevice {
		if err := h.issueTrustedDevice(r.Context(), w, r, u.ID); err != nil {
			util.WriteError(w, 500, "totp_confirm_failed", "cannot remember trusted device", middleware.RequestID(r.Context()))
			return
		}
	}
	_ = h.svc.Store().DeleteSetting(r.Context(), mfaTrustedPendingRememberSettingKey(sess.ID))
	_ = h.svc.Store().SetLegacyMFAPromptPending(r.Context(), u.ID, false)
	u.MFABackupCompleted = true
	now := time.Now().UTC()
	sess.MFAVerifiedAt = &now
	stage, err := h.svc.ResolveMFAStage(r.Context(), u, &sess)
	if err != nil {
		util.WriteError(w, 500, "totp_confirm_failed", "cannot resolve mfa stage", middleware.RequestID(r.Context()))
		return
	}
	out := map[string]any{"status": "enabled", "verified": true}
	applyAuthStageFields(out, stage)
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2MFAWebAuthnRegisterBegin(w http.ResponseWriter, r *http.Request) {
	if !h.mailSecRuntimeEnabled() {
		util.WriteError(w, http.StatusServiceUnavailable, "mailsec_unavailable", "mailsec service is required for webauthn", middleware.RequestID(r.Context()))
		return
	}
	u, _ := middleware.User(r.Context())
	creds, err := h.svc.Store().ListMFAWebAuthnCredentials(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "webauthn_register_begin_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	webAuthnContext, ok := h.requireWebAuthnContext(w, r)
	if !ok {
		return
	}
	rpID, origins := webAuthnContext.RPID, webAuthnContext.Origins
	challenge, err := randomToken()
	if err != nil {
		util.WriteError(w, 500, "webauthn_register_begin_failed", "cannot generate challenge", middleware.RequestID(r.Context()))
		return
	}
	state := webAuthnChallengeState{
		UserID:    u.ID,
		Mode:      "register",
		Challenge: challenge,
		ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
		RPID:      rpID,
		Origins:   origins,
	}
	if err := h.storeWebAuthnChallenge(r.Context(), webAuthnRegisterChallengeSettingKey(u.ID), state); err != nil {
		util.WriteError(w, 500, "webauthn_register_begin_failed", "cannot persist challenge", middleware.RequestID(r.Context()))
		return
	}
	excludeCredentials := make([]map[string]any, 0, len(creds))
	for _, cred := range creds {
		excludeCredentials = append(excludeCredentials, map[string]any{
			"id":         cred.CredentialID,
			"type":       "public-key",
			"transports": parseWebAuthnTransportsJSON(cred.TransportsJSON),
		})
	}
	util.WriteJSON(w, 200, map[string]any{
		"status":     "challenge_created",
		"challenge":  challenge,
		"timeout_ms": 300000,
		"rp": map[string]any{
			"id":   rpID,
			"name": "Despatch",
		},
		"user": map[string]any{
			"id":           hex.EncodeToString([]byte(u.ID)),
			"name":         u.Email,
			"display_name": u.Email,
		},
		"pub_key_cred_params": []map[string]any{
			{"type": "public-key", "alg": -7},
			{"type": "public-key", "alg": -257},
		},
		"public_key": map[string]any{
			"challenge":              challenge,
			"rp":                     map[string]any{"id": rpID, "name": "Despatch"},
			"user":                   map[string]any{"id": hex.EncodeToString([]byte(u.ID)), "name": u.Email, "displayName": u.Email},
			"pubKeyCredParams":       []map[string]any{{"type": "public-key", "alg": -7}, {"type": "public-key", "alg": -257}},
			"excludeCredentials":     excludeCredentials,
			"attestation":            "none",
			"timeout":                300000,
			"authenticatorSelection": map[string]any{"userVerification": "preferred"},
		},
		"exclude_credentials": excludeCredentials,
	})
}

func (h *Handlers) V2MFAWebAuthnRegisterFinish(w http.ResponseWriter, r *http.Request) {
	if !h.mailSecRuntimeEnabled() {
		util.WriteError(w, http.StatusServiceUnavailable, "mailsec_unavailable", "mailsec service is required for webauthn", middleware.RequestID(r.Context()))
		return
	}
	webAuthnContext, ok := h.requireWebAuthnContext(w, r)
	if !ok {
		return
	}
	u, _ := middleware.User(r.Context())
	sess, ok := middleware.Session(r.Context())
	if !ok {
		util.WriteError(w, http.StatusUnauthorized, "session_missing", "authentication required", middleware.RequestID(r.Context()))
		return
	}
	var req map[string]any
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	rememberDevice, _ := getBoolValue(req, "remember_device")
	resp := getMapValue(req, "response")
	clientDataJSON := getStringValue(resp, "client_data_json", "clientDataJSON")
	attestationObject := getStringValue(resp, "attestation_object", "attestationObject")
	if clientDataJSON == "" || attestationObject == "" {
		util.WriteError(w, 400, "bad_request", "response.clientDataJSON and response.attestationObject are required", middleware.RequestID(r.Context()))
		return
	}
	credentialID := getStringValue(req, "credential_id", "raw_id", "rawId", "id")
	challenge := getStringValue(req, "challenge")
	state, ok, err := h.loadWebAuthnChallenge(r.Context(), webAuthnRegisterChallengeSettingKey(u.ID))
	if err != nil {
		util.WriteError(w, 500, "webauthn_register_finish_failed", "cannot read challenge state", middleware.RequestID(r.Context()))
		return
	}
	if !ok || state.Mode != "register" || state.UserID != u.ID || time.Now().UTC().After(state.ExpiresAt) {
		util.WriteError(w, 401, "webauthn_challenge_invalid", "challenge is missing or expired", middleware.RequestID(r.Context()))
		return
	}
	if challenge != "" && subtleConstantCompare(state.Challenge, challenge) != 1 {
		util.WriteError(w, 401, "webauthn_challenge_invalid", "challenge mismatch", middleware.RequestID(r.Context()))
		return
	}
	mailsecResult, err := h.callMailSecOperation(r.Context(), "webauthn.register.finish", u.ID, map[string]any{
		"challenge":                 state.Challenge,
		"rp_id":                     firstNonEmpty(state.RPID, webAuthnContext.RPID),
		"origins":                   firstNonEmptySlice(state.Origins, webAuthnContext.Origins),
		"client_data_json_b64url":   clientDataJSON,
		"attestation_object_b64url": attestationObject,
		"credential_id":             credentialID,
		"require_user_verification": false,
	})
	if err != nil {
		util.WriteError(w, http.StatusUnauthorized, "webauthn_register_finish_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	resultCredentialID := getStringValue(mailsecResult, "credential_id")
	publicKey := getStringValue(mailsecResult, "public_key_cose_b64url")
	signCount, signCountOK := getInt64Value(mailsecResult, "sign_count")
	if resultCredentialID == "" || publicKey == "" || !signCountOK || signCount < 0 {
		util.WriteError(w, 500, "webauthn_register_finish_failed", "mailsec response is incomplete", middleware.RequestID(r.Context()))
		return
	}
	transports := getStringSliceValue(req, "transports")
	if len(transports) == 0 {
		transports = getStringSliceValue(resp, "transports")
	}
	transportsJSON := "[]"
	if len(transports) > 0 {
		b, err := json.Marshal(transports)
		if err != nil {
			util.WriteError(w, 400, "bad_request", "invalid transports", middleware.RequestID(r.Context()))
			return
		}
		transportsJSON = string(b)
	}
	name := getStringValue(req, "name")
	if name == "" {
		name = "Passkey"
	}
	created, err := h.svc.Store().UpsertMFAWebAuthnCredential(r.Context(), models.MFAWebAuthnCredential{
		ID:             uuid.NewString(),
		UserID:         u.ID,
		CredentialID:   resultCredentialID,
		PublicKey:      publicKey,
		SignCount:      signCount,
		TransportsJSON: transportsJSON,
		Name:           name,
		CreatedAt:      time.Now().UTC(),
	})
	if err != nil {
		util.WriteError(w, 500, "webauthn_register_finish_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	registerMeta, _ := json.Marshal(map[string]any{
		"credential":    created.ID,
		"credential_id": created.CredentialID,
		"name":          created.Name,
	})
	_ = h.svc.Store().InsertAudit(r.Context(), u.ID, "auth.passkey.register", u.ID, string(registerMeta))

	if err := h.svc.Store().SetUserMFABackupCompleted(r.Context(), u.ID, false); err != nil {
		util.WriteError(w, 500, "webauthn_register_finish_failed", "cannot update mfa backup state", middleware.RequestID(r.Context()))
		return
	}

	recoveryCodes, err := generateRecoveryCodes(10)
	if err != nil {
		util.WriteError(w, 500, "webauthn_register_finish_failed", "cannot generate recovery codes", middleware.RequestID(r.Context()))
		return
	}
	hashes := make([]string, 0, len(recoveryCodes))
	for _, code := range recoveryCodes {
		sum := sha256.Sum256([]byte(code))
		hashes = append(hashes, hex.EncodeToString(sum[:]))
	}
	if err := h.svc.Store().ReplaceMFARecoveryCodes(r.Context(), u.ID, hashes); err != nil {
		util.WriteError(w, 500, "webauthn_register_finish_failed", "cannot persist recovery codes", middleware.RequestID(r.Context()))
		return
	}

	if rememberDevice {
		if err := h.svc.Store().UpsertSetting(r.Context(), mfaTrustedPendingRememberSettingKey(sess.ID), "1"); err != nil {
			util.WriteError(w, 500, "webauthn_register_finish_failed", "cannot persist remember-device state", middleware.RequestID(r.Context()))
			return
		}
	} else {
		_ = h.svc.Store().DeleteSetting(r.Context(), mfaTrustedPendingRememberSettingKey(sess.ID))
	}

	_ = h.svc.Store().DeleteSetting(r.Context(), webAuthnRegisterChallengeSettingKey(u.ID))
	u.MFABackupCompleted = false
	stage, err := h.svc.ResolveMFAStage(r.Context(), u, &sess)
	if err != nil {
		util.WriteError(w, 500, "webauthn_register_finish_failed", "cannot resolve mfa stage", middleware.RequestID(r.Context()))
		return
	}
	setNoStoreHeaders(w)
	out := map[string]any{
		"status":              "backup_ack_required",
		"backup_ack_required": true,
		"credential":          created,
		"recovery_codes":      recoveryCodes,
		"remember_device":     rememberDevice,
		"mfa_setup_step":      service.MFASetupStepBackup,
		"setup_instructions": []string{
			"Your passkey is ready.",
			"Save your recovery codes now.",
			"Confirm that your recovery codes are saved to finish MFA setup.",
		},
	}
	applyAuthStageFields(out, stage)
	util.WriteJSON(w, 201, out)
}

func (h *Handlers) V2MFARecoveryCodesAck(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	sess, ok := middleware.Session(r.Context())
	if !ok {
		util.WriteError(w, http.StatusUnauthorized, "session_missing", "authentication required", middleware.RequestID(r.Context()))
		return
	}
	var req struct {
		RecoveryCodesAck bool `json:"recovery_codes_ack"`
		RememberDevice   bool `json:"remember_device"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if !req.RecoveryCodesAck {
		util.WriteError(w, 400, "recovery_codes_ack_required", "you must confirm that recovery codes were saved", middleware.RequestID(r.Context()))
		return
	}
	status, err := h.svc.Store().GetMFAStatus(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "mfa_backup_ack_failed", "cannot load mfa status", middleware.RequestID(r.Context()))
		return
	}
	if !status.TOTPEnabled && status.WebAuthnCount == 0 {
		util.WriteError(w, 409, "mfa_primary_factor_missing", "set up a primary MFA method before confirming recovery codes", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().SetUserMFABackupCompleted(r.Context(), u.ID, true); err != nil {
		util.WriteError(w, 500, "mfa_backup_ack_failed", "cannot update mfa backup state", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().SetSessionMFAVerified(r.Context(), sess.ID, "mfa_backup_ack"); err != nil {
		util.WriteError(w, 500, "mfa_backup_ack_failed", "cannot mark session mfa state", middleware.RequestID(r.Context()))
		return
	}
	pendingRemember := false
	if raw, ok, err := h.svc.Store().GetSetting(r.Context(), mfaTrustedPendingRememberSettingKey(sess.ID)); err == nil && ok && strings.TrimSpace(raw) == "1" {
		pendingRemember = true
	}
	rememberDevice := req.RememberDevice || pendingRemember
	if rememberDevice {
		if err := h.issueTrustedDevice(r.Context(), w, r, u.ID); err != nil {
			util.WriteError(w, 500, "mfa_backup_ack_failed", "cannot remember trusted device", middleware.RequestID(r.Context()))
			return
		}
	}
	_ = h.svc.Store().DeleteSetting(r.Context(), mfaTrustedPendingRememberSettingKey(sess.ID))
	_ = h.svc.Store().SetLegacyMFAPromptPending(r.Context(), u.ID, false)
	u.MFABackupCompleted = true
	now := time.Now().UTC()
	sess.MFAVerifiedAt = &now
	stage, err := h.svc.ResolveMFAStage(r.Context(), u, &sess)
	if err != nil {
		util.WriteError(w, 500, "mfa_backup_ack_failed", "cannot resolve mfa stage", middleware.RequestID(r.Context()))
		return
	}
	out := map[string]any{"status": "ok", "verified": true}
	applyAuthStageFields(out, stage)
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2MFALegacyDismiss(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	sess, _ := middleware.Session(r.Context())
	if err := h.svc.Store().SetLegacyMFAPromptPending(r.Context(), u.ID, false); err != nil {
		util.WriteError(w, 500, "mfa_legacy_dismiss_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	u.LegacyMFAPromptPending = false
	stage, err := h.svc.ResolveMFAStage(r.Context(), u, &sess)
	if err != nil {
		util.WriteError(w, 500, "mfa_legacy_dismiss_failed", "cannot resolve mfa stage", middleware.RequestID(r.Context()))
		return
	}
	out := map[string]any{"status": "dismissed"}
	applyAuthStageFields(out, stage)
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2MFAUpdatePreference(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	sess, _ := middleware.Session(r.Context())
	var req struct {
		Preference string `json:"preference"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	target := service.NormalizeMFAPreference(req.Preference)
	if target != service.MFAPreferenceTOTP && target != service.MFAPreferenceWebAuthn {
		util.WriteError(w, 400, "bad_request", "preference must be totp or webauthn", middleware.RequestID(r.Context()))
		return
	}
	stage, err := h.svc.ResolveMFAStage(r.Context(), u, &sess)
	if err != nil {
		util.WriteError(w, 500, "mfa_preference_update_failed", "cannot resolve mfa stage", middleware.RequestID(r.Context()))
		return
	}
	if !stage.MFASetupRequired {
		util.WriteError(w, 409, "mfa_preference_switch_not_allowed", "preference switch is only available during required setup", middleware.RequestID(r.Context()))
		return
	}
	if stage.MFAPreference == target {
		out := map[string]any{"status": "unchanged"}
		applyAuthStageFields(out, stage)
		util.WriteJSON(w, 200, out)
		return
	}
	if stage.MFAPreference != service.MFAPreferenceTOTP && stage.MFAPreference != service.MFAPreferenceWebAuthn {
		util.WriteError(w, 409, "mfa_preference_switch_not_allowed", "preference switch is only supported between totp and webauthn", middleware.RequestID(r.Context()))
		return
	}
	if u.MFASetupSwitchUsed {
		util.WriteError(w, 409, "mfa_preference_switch_exhausted", "mfa setup preference switch has already been used", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().UpdateUserMFAPreference(r.Context(), u.ID, target); err != nil {
		util.WriteError(w, 500, "mfa_preference_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().MarkMFASetupSwitchUsed(r.Context(), u.ID); err != nil {
		util.WriteError(w, 500, "mfa_preference_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	updatedUser, err := h.svc.Store().GetUserByID(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "mfa_preference_update_failed", "cannot load updated user preference", middleware.RequestID(r.Context()))
		return
	}
	nextStage, err := h.svc.ResolveMFAStage(r.Context(), updatedUser, &sess)
	if err != nil {
		util.WriteError(w, 500, "mfa_preference_update_failed", "cannot resolve mfa stage", middleware.RequestID(r.Context()))
		return
	}
	out := map[string]any{
		"status":     "updated",
		"preference": updatedUser.MFAPreference,
	}
	applyAuthStageFields(out, nextStage)
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2MFAWebAuthnDelete(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		util.WriteError(w, 400, "bad_request", "credential id is required", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().DeleteMFAWebAuthnCredential(r.Context(), u.ID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "credential_not_found", "webauthn credential not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "webauthn_delete_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	meta, _ := json.Marshal(map[string]any{"credential": id})
	_ = h.svc.Store().InsertAudit(r.Context(), u.ID, "auth.passkey.delete", u.ID, string(meta))
	util.WriteJSON(w, 200, map[string]any{"status": "deleted"})
}

func (h *Handlers) V2MFAWebAuthnList(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	items, err := h.svc.Store().ListMFAWebAuthnCredentials(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "webauthn_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2MFAWebAuthnRename(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		util.WriteError(w, 400, "bad_request", "credential id is required", middleware.RequestID(r.Context()))
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		util.WriteError(w, 400, "bad_request", "credential name is required", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().RenameMFAWebAuthnCredential(r.Context(), u.ID, id, req.Name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "credential_not_found", "webauthn credential not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "webauthn_rename_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "ok"})
}

func (h *Handlers) V2ListTrustedDevices(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	items, err := h.svc.Store().ListActiveMFATrustedDevices(r.Context(), u.ID, time.Now().UTC())
	if err != nil {
		util.WriteError(w, 500, "trusted_devices_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	currentDeviceID := ""
	if cookie, err := r.Cookie(h.trustedDeviceCookieName()); err == nil && strings.TrimSpace(cookie.Value) != "" {
		current, err := h.svc.Store().GetActiveMFATrustedDeviceByTokenHash(r.Context(), u.ID, trustedDeviceTokenHash(cookie.Value), time.Now().UTC())
		if err == nil {
			currentDeviceID = current.ID
		}
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		displayLabel, browser, osLabel, deviceType := summarizeTrustedDeviceLabel(item.DeviceLabel)
		entry := map[string]any{
			"id":            item.ID,
			"user_id":       item.UserID,
			"ip_hint":       item.IPHint,
			"device_label":  item.DeviceLabel,
			"created_at":    item.CreatedAt,
			"expires_at":    item.ExpiresAt,
			"is_current":    currentDeviceID != "" && item.ID == currentDeviceID,
			"display_label": displayLabel,
			"browser":       browser,
			"os":            osLabel,
			"device_type":   deviceType,
		}
		if !item.LastUsedAt.IsZero() {
			entry["last_used_at"] = item.LastUsedAt
		}
		if !item.RevokedAt.IsZero() {
			entry["revoked_at"] = item.RevokedAt
		}
		out = append(out, entry)
	}
	util.WriteJSON(w, 200, map[string]any{"items": out})
}

func (h *Handlers) V2RevokeTrustedDevice(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	targetID := strings.TrimSpace(chi.URLParam(r, "id"))
	if targetID == "" {
		util.WriteError(w, 400, "bad_request", "trusted device id is required", middleware.RequestID(r.Context()))
		return
	}
	activeCookieDeviceID := ""
	if cookie, err := r.Cookie(h.trustedDeviceCookieName()); err == nil && strings.TrimSpace(cookie.Value) != "" {
		current, err := h.svc.Store().GetActiveMFATrustedDeviceByTokenHash(r.Context(), u.ID, trustedDeviceTokenHash(cookie.Value), time.Now().UTC())
		if err == nil {
			activeCookieDeviceID = current.ID
		}
	}
	if err := h.svc.Store().RevokeMFATrustedDevice(r.Context(), u.ID, targetID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "trusted_device_not_found", "trusted device not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "trusted_device_revoke_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if activeCookieDeviceID != "" && activeCookieDeviceID == targetID {
		h.clearTrustedDeviceCookie(w, r)
	}
	util.WriteJSON(w, 200, map[string]any{"status": "revoked", "id": targetID})
}

func (h *Handlers) V2RevokeAllTrustedDevices(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	if err := h.svc.Store().RevokeAllMFATrustedDevices(r.Context(), u.ID); err != nil {
		util.WriteError(w, 500, "trusted_device_revoke_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	h.clearTrustedDeviceCookie(w, r)
	util.WriteJSON(w, 200, map[string]any{"status": "revoked_all"})
}

func (h *Handlers) V2ListSessions(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	sess, _ := middleware.Session(r.Context())
	items, err := h.svc.Store().ListSessionsMeta(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "sessions_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	currentSessionID := strings.TrimSpace(sess.ID)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry := map[string]any{
			"session_id":      item.SessionID,
			"user_id":         item.UserID,
			"device_label":    item.DeviceLabel,
			"ua_summary":      item.UASummary,
			"ip_hint":         item.IPHint,
			"auth_method":     item.AuthMethod,
			"created_at":      item.CreatedAt,
			"last_seen_at":    item.LastSeenAt,
			"expires_at":      item.ExpiresAt,
			"idle_expires_at": item.IdleExpiresAt,
			"is_current":      currentSessionID != "" && item.SessionID == currentSessionID,
		}
		if !item.MFAVerifiedAt.IsZero() {
			entry["mfa_verified_at"] = item.MFAVerifiedAt
		}
		if !item.RevokedAt.IsZero() {
			entry["revoked_at"] = item.RevokedAt
		}
		if strings.TrimSpace(item.RevokedReason) != "" {
			entry["revoked_reason"] = item.RevokedReason
		}
		out = append(out, entry)
	}
	util.WriteJSON(w, 200, map[string]any{"items": out})
}

func (h *Handlers) V2RevokeSession(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := chi.URLParam(r, "id")
	var req struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, true); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if err := h.svc.Store().RevokeSessionWithReason(r.Context(), u.ID, id, strings.TrimSpace(req.Reason)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "session_not_found", "session not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "session_revoke_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "revoked"})
}

func (h *Handlers) V2ListCryptoKeyrings(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	items, err := h.svc.Store().ListCryptoKeyrings(r.Context(), u.ID, accountID, kind)
	if err != nil {
		util.WriteError(w, 500, "crypto_keyrings_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	for i := range items {
		items[i].PrivateKeyEnc = ""
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2CreateCryptoKeyring(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req struct {
		AccountID      string   `json:"account_id"`
		Kind           string   `json:"kind"`
		Fingerprint    string   `json:"fingerprint"`
		UserIDs        []string `json:"user_ids"`
		PublicKey      string   `json:"public_key"`
		PrivateKey     string   `json:"private_key"`
		PassphraseHint string   `json:"passphrase_hint"`
		TrustLevel     string   `json:"trust_level"`
		ExpiresAt      string   `json:"expires_at"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitLarge, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	item, err := h.buildCryptoKeyringFromRequest(
		r.Context(),
		u.ID,
		"",
		req.AccountID,
		req.Kind,
		req.Fingerprint,
		req.UserIDs,
		req.PublicKey,
		req.PrivateKey,
		req.PassphraseHint,
		req.TrustLevel,
		req.ExpiresAt,
	)
	if err != nil {
		util.WriteError(w, 400, "crypto_keyring_invalid", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	item.ID = uuid.NewString()
	out, err := h.svc.Store().CreateCryptoKeyring(r.Context(), item)
	if err != nil {
		util.WriteError(w, 500, "crypto_keyring_create_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	out.PrivateKeyEnc = ""
	util.WriteJSON(w, 201, out)
}

func (h *Handlers) V2UpdateCryptoKeyring(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	current, err := h.svc.Store().GetCryptoKeyringByID(r.Context(), u.ID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "crypto_keyring_not_found", "crypto keyring not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "crypto_keyring_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req struct {
		AccountID      *string   `json:"account_id"`
		Kind           *string   `json:"kind"`
		Fingerprint    *string   `json:"fingerprint"`
		UserIDs        *[]string `json:"user_ids"`
		PublicKey      *string   `json:"public_key"`
		PrivateKey     *string   `json:"private_key"`
		PassphraseHint *string   `json:"passphrase_hint"`
		TrustLevel     *string   `json:"trust_level"`
		ExpiresAt      *string   `json:"expires_at"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitLarge, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}

	accountID := current.AccountID
	if req.AccountID != nil {
		accountID = strings.TrimSpace(*req.AccountID)
	}
	kind := current.Kind
	if req.Kind != nil {
		kind = strings.TrimSpace(*req.Kind)
	}
	fingerprint := current.Fingerprint
	if req.Fingerprint != nil {
		fingerprint = strings.TrimSpace(*req.Fingerprint)
	}
	userIDs := parseJSONStringSlice(current.UserIDsJSON)
	if req.UserIDs != nil {
		userIDs = *req.UserIDs
	}
	publicKey := current.PublicKey
	if req.PublicKey != nil {
		publicKey = *req.PublicKey
	}
	privateKey := ""
	if req.PrivateKey != nil {
		privateKey = *req.PrivateKey
	}
	passphraseHint := current.PassphraseHint
	if req.PassphraseHint != nil {
		passphraseHint = *req.PassphraseHint
	}
	trustLevel := current.TrustLevel
	if req.TrustLevel != nil {
		trustLevel = *req.TrustLevel
	}
	expiresAt := ""
	if !current.ExpiresAt.IsZero() {
		expiresAt = current.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if req.ExpiresAt != nil {
		expiresAt = strings.TrimSpace(*req.ExpiresAt)
	}

	item, err := h.buildCryptoKeyringFromRequest(
		r.Context(),
		u.ID,
		id,
		accountID,
		kind,
		fingerprint,
		userIDs,
		publicKey,
		privateKey,
		passphraseHint,
		trustLevel,
		expiresAt,
	)
	if err != nil {
		util.WriteError(w, 400, "crypto_keyring_invalid", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if strings.TrimSpace(privateKey) == "" {
		item.PrivateKeyEnc = current.PrivateKeyEnc
	}
	out, err := h.svc.Store().UpdateCryptoKeyring(r.Context(), item)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "crypto_keyring_not_found", "crypto keyring not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "crypto_keyring_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	out.PrivateKeyEnc = ""
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2DeleteCryptoKeyring(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		util.WriteError(w, 400, "bad_request", "id is required", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().DeleteCryptoKeyring(r.Context(), u.ID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "crypto_keyring_not_found", "crypto keyring not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "crypto_keyring_delete_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "deleted"})
}

func (h *Handlers) V2ListCryptoTrustPolicies(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	items, err := h.svc.Store().ListCryptoTrustPolicies(r.Context(), u.ID, accountID)
	if err != nil {
		util.WriteError(w, 500, "crypto_trust_policies_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2CreateCryptoTrustPolicy(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req models.CryptoTrustPolicy
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	req.ID = uuid.NewString()
	req.UserID = u.ID
	out, err := h.svc.Store().CreateCryptoTrustPolicy(r.Context(), req)
	if err != nil {
		util.WriteError(w, 500, "crypto_trust_policy_create_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 201, out)
}

func (h *Handlers) V2UpdateCryptoTrustPolicy(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	var req models.CryptoTrustPolicy
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	req.ID = id
	req.UserID = u.ID
	out, err := h.svc.Store().UpdateCryptoTrustPolicy(r.Context(), req)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "crypto_trust_policy_not_found", "crypto trust policy not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "crypto_trust_policy_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2DeleteCryptoTrustPolicy(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		util.WriteError(w, 400, "bad_request", "id is required", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().DeleteCryptoTrustPolicy(r.Context(), u.ID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "crypto_trust_policy_not_found", "crypto trust policy not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "crypto_trust_policy_delete_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "deleted"})
}

func (h *Handlers) V2GetQuota(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" {
		util.WriteError(w, 400, "bad_request", "account_id is required", middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	item, err := h.svc.Store().GetQuotaCacheByAccount(r.Context(), accountID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteJSON(w, 200, map[string]any{
				"account_id":      accountID,
				"used_bytes":      0,
				"total_bytes":     0,
				"used_messages":   0,
				"total_messages":  0,
				"refreshed_at":    time.Now().UTC(),
				"quota_available": false,
			})
			return
		}
		util.WriteError(w, 500, "quota_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{
		"account_id":      item.AccountID,
		"used_bytes":      item.UsedBytes,
		"total_bytes":     item.TotalBytes,
		"used_messages":   item.UsedMessages,
		"total_messages":  item.TotalMessages,
		"refreshed_at":    item.RefreshedAt,
		"last_error":      item.LastError,
		"quota_available": true,
	})
}

func (h *Handlers) v2SendWithAccount(ctx context.Context, u models.User, accountID string, req mail.SendRequest) (mail.SendResult, error) {
	if strings.TrimSpace(req.From) == "" {
		req.From = u.Email
	}
	if strings.TrimSpace(accountID) == "" {
		pass, err := h.sessionMailPasswordFromContext(ctx)
		if err != nil {
			return mail.SendResult{}, err
		}
		login := service.MailIdentity(u)
		return h.svc.Mail().Send(ctx, login, pass, req)
	}
	acc, err := h.svc.Store().GetMailAccountByID(ctx, u.ID, accountID)
	if err != nil {
		return mail.SendResult{}, err
	}
	pass, err := util.DecryptString(util.Derive32ByteKey(h.cfg.SessionEncryptKey), acc.SecretEnc)
	if err != nil {
		return mail.SendResult{}, fmt.Errorf("cannot decrypt account secret")
	}
	cfg := h.cfg
	cfg.IMAPHost = acc.IMAPHost
	cfg.IMAPPort = acc.IMAPPort
	cfg.IMAPTLS = acc.IMAPTLS
	cfg.IMAPStartTLS = acc.IMAPStartTLS
	cfg.SMTPHost = acc.SMTPHost
	cfg.SMTPPort = acc.SMTPPort
	cfg.SMTPTLS = acc.SMTPTLS
	cfg.SMTPStartTLS = acc.SMTPStartTLS
	cli := mail.NewIMAPSMTPClient(cfg)
	return cli.Send(ctx, acc.Login, pass, req)
}

type v2SendCryptoOptions struct {
	Provider            string   `json:"provider"`
	Sign                bool     `json:"sign"`
	Encrypt             bool     `json:"encrypt"`
	SignKeyringID       string   `json:"sign_keyring_id"`
	RecipientKeyringIDs []string `json:"recipient_keyring_ids"`
}

func (h *Handlers) applyCryptoToSendRequest(ctx context.Context, u models.User, accountID string, req mail.SendRequest, cryptoOptionsJSON, cryptoPassphrase string) (mail.SendRequest, error) {
	cryptoOptionsJSON = strings.TrimSpace(cryptoOptionsJSON)
	if cryptoOptionsJSON == "" || cryptoOptionsJSON == "{}" {
		return req, nil
	}
	if !h.mailSecRuntimeEnabled() {
		return req, fmt.Errorf("mailsec service is required for crypto operations")
	}
	var opts v2SendCryptoOptions
	if err := json.Unmarshal([]byte(cryptoOptionsJSON), &opts); err != nil {
		return req, fmt.Errorf("invalid crypto options: %w", err)
	}
	if !opts.Sign && !opts.Encrypt {
		return req, nil
	}
	provider := strings.ToLower(strings.TrimSpace(opts.Provider))
	body := req.Body

	if opts.Sign {
		keyID := strings.TrimSpace(opts.SignKeyringID)
		if keyID == "" {
			return req, fmt.Errorf("sign_keyring_id is required for signing")
		}
		signKey, err := h.svc.Store().GetCryptoKeyringByID(ctx, u.ID, keyID)
		if err != nil {
			return req, fmt.Errorf("cannot load signing keyring: %w", err)
		}
		normalizedKind := normalizeCryptoKeyringKind(signKey.Kind)
		if !strings.HasSuffix(normalizedKind, "_private") {
			return req, fmt.Errorf("signing keyring must be a private keyring")
		}
		inferredProvider := inferCryptoProvider(normalizedKind)
		if provider == "" {
			provider = inferredProvider
		} else if provider != inferredProvider {
			return req, fmt.Errorf("provider does not match signing keyring kind")
		}
		if strings.TrimSpace(signKey.PrivateKeyEnc) == "" {
			return req, fmt.Errorf("signing keyring has no private key material")
		}
		privateKey, err := util.DecryptString(util.Derive32ByteKey(h.cfg.SessionEncryptKey), signKey.PrivateKeyEnc)
		if err != nil {
			return req, fmt.Errorf("cannot decrypt signing key")
		}
		var op string
		payload := map[string]any{}
		switch provider {
		case "pgp":
			op = "crypto.pgp.sign"
			payload = map[string]any{
				"plaintext":           body,
				"private_key_armored": privateKey,
				"passphrase":          cryptoPassphrase,
			}
		case "smime":
			if strings.TrimSpace(signKey.PublicKey) == "" {
				return req, fmt.Errorf("smime signing requires certificate in public_key")
			}
			op = "crypto.smime.sign"
			payload = map[string]any{
				"plaintext":              body,
				"private_key_pem":        privateKey,
				"cert_pem":               signKey.PublicKey,
				"private_key_passphrase": cryptoPassphrase,
			}
		default:
			return req, fmt.Errorf("unsupported crypto provider: %s", provider)
		}
		result, err := h.callMailSecOperation(ctx, op, accountID, payload)
		if err != nil {
			return req, err
		}
		switch provider {
		case "pgp":
			next := getStringValue(result, "signed_message_armored")
			if next == "" {
				next = getStringValue(result, "signed_message_b64url")
			}
			if next == "" {
				return req, fmt.Errorf("mailsec signed message missing payload")
			}
			body = next
		case "smime":
			next := getStringValue(result, "signed_smime")
			if next == "" {
				return req, fmt.Errorf("mailsec signed smime missing payload")
			}
			body = next
		}
	}

	if opts.Encrypt {
		if len(opts.RecipientKeyringIDs) == 0 {
			return req, fmt.Errorf("recipient_keyring_ids is required for encryption")
		}
		recipientKeyrings := make([]models.CryptoKeyring, 0, len(opts.RecipientKeyringIDs))
		for _, keyID := range opts.RecipientKeyringIDs {
			trimmedID := strings.TrimSpace(keyID)
			if trimmedID == "" {
				return req, fmt.Errorf("recipient_keyring_ids contains empty id")
			}
			item, err := h.svc.Store().GetCryptoKeyringByID(ctx, u.ID, trimmedID)
			if err != nil {
				return req, fmt.Errorf("cannot load recipient keyring %q: %w", keyID, err)
			}
			if strings.TrimSpace(item.PublicKey) == "" {
				return req, fmt.Errorf("recipient keyring %q has no public key material", keyID)
			}
			itemProvider := inferCryptoProvider(normalizeCryptoKeyringKind(item.Kind))
			if provider == "" {
				provider = itemProvider
			} else if provider != itemProvider {
				return req, fmt.Errorf("recipient keyring %q provider mismatch", keyID)
			}
			recipientKeyrings = append(recipientKeyrings, item)
		}
		var op string
		payload := map[string]any{}
		switch provider {
		case "pgp":
			publicKeys := make([]string, 0, len(recipientKeyrings))
			for _, keyring := range recipientKeyrings {
				publicKeys = append(publicKeys, keyring.PublicKey)
			}
			op = "crypto.pgp.encrypt"
			payload = map[string]any{
				"plaintext":             body,
				"recipient_public_keys": publicKeys,
			}
		case "smime":
			certs := make([]string, 0, len(recipientKeyrings))
			for _, keyring := range recipientKeyrings {
				certs = append(certs, keyring.PublicKey)
			}
			op = "crypto.smime.encrypt"
			payload = map[string]any{
				"plaintext":           body,
				"recipient_certs_pem": certs,
			}
		default:
			return req, fmt.Errorf("unsupported crypto provider: %s", provider)
		}
		result, err := h.callMailSecOperation(ctx, op, accountID, payload)
		if err != nil {
			return req, err
		}
		switch provider {
		case "pgp":
			next := getStringValue(result, "ciphertext_armored")
			if next == "" {
				next = getStringValue(result, "ciphertext_b64url")
			}
			if next == "" {
				return req, fmt.Errorf("mailsec encrypted message missing payload")
			}
			body = next
		case "smime":
			next := getStringValue(result, "ciphertext_smime")
			if next == "" {
				return req, fmt.Errorf("mailsec encrypted smime missing payload")
			}
			body = next
		}
	}

	req.Body = body
	return req, nil
}

func (h *Handlers) buildCryptoKeyringFromRequest(
	ctx context.Context,
	userID, id, accountID, kind, fingerprint string,
	userIDs []string,
	publicKey, privateKey, passphraseHint, trustLevel, expiresAtRaw string,
) (models.CryptoKeyring, error) {
	kind = normalizeCryptoKeyringKind(kind)
	if kind == "" {
		return models.CryptoKeyring{}, fmt.Errorf("kind must be one of: pgp_public, pgp_private, smime_public, smime_private")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID != "" {
		if _, err := h.svc.Store().GetMailAccountByID(ctx, userID, accountID); err != nil {
			return models.CryptoKeyring{}, fmt.Errorf("account does not belong to current user")
		}
	}
	publicKey = strings.TrimSpace(publicKey)
	privateKey = strings.TrimSpace(privateKey)
	if publicKey == "" && privateKey == "" {
		return models.CryptoKeyring{}, fmt.Errorf("public_key or private_key is required")
	}
	if strings.Contains(kind, "public") && publicKey == "" {
		return models.CryptoKeyring{}, fmt.Errorf("public_key is required for public keyring kind")
	}
	if strings.Contains(kind, "private") && privateKey == "" && id == "" {
		return models.CryptoKeyring{}, fmt.Errorf("private_key is required for private keyring kind")
	}
	if kind == "smime_private" && publicKey == "" {
		return models.CryptoKeyring{}, fmt.Errorf("public_key certificate is required for smime_private")
	}
	privateKeyEnc := ""
	if privateKey != "" {
		enc, err := util.EncryptString(util.Derive32ByteKey(h.cfg.SessionEncryptKey), privateKey)
		if err != nil {
			return models.CryptoKeyring{}, fmt.Errorf("cannot encrypt private key")
		}
		privateKeyEnc = enc
	}
	if strings.TrimSpace(fingerprint) == "" {
		fingerprint = deriveCryptoFingerprint(firstNonEmpty(publicKey, privateKey))
	}
	userIDsJSON := "[]"
	if len(userIDs) > 0 {
		b, err := json.Marshal(userIDs)
		if err != nil {
			return models.CryptoKeyring{}, fmt.Errorf("invalid user_ids")
		}
		userIDsJSON = string(b)
	}
	expiresAt := time.Time{}
	if strings.TrimSpace(expiresAtRaw) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(expiresAtRaw))
		if err != nil {
			return models.CryptoKeyring{}, fmt.Errorf("expires_at must be RFC3339")
		}
		expiresAt = parsed.UTC()
	}
	trustLevel = strings.TrimSpace(strings.ToLower(trustLevel))
	if trustLevel == "" {
		trustLevel = "unknown"
	}
	return models.CryptoKeyring{
		ID:             id,
		UserID:         userID,
		AccountID:      accountID,
		Kind:           kind,
		Fingerprint:    strings.ToUpper(strings.TrimSpace(fingerprint)),
		UserIDsJSON:    userIDsJSON,
		PublicKey:      publicKey,
		PrivateKeyEnc:  privateKeyEnc,
		PassphraseHint: strings.TrimSpace(passphraseHint),
		ExpiresAt:      expiresAt,
		TrustLevel:     trustLevel,
	}, nil
}

func normalizeCryptoKeyringKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "pgp_public", "pgp-private-public", "pgp-public":
		return "pgp_public"
	case "pgp_private", "pgp-private":
		return "pgp_private"
	case "smime_public", "smime-public":
		return "smime_public"
	case "smime_private", "smime-private":
		return "smime_private"
	default:
		return ""
	}
}

func inferCryptoProvider(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch {
	case strings.HasPrefix(kind, "smime"):
		return "smime"
	default:
		return "pgp"
	}
}

func parseJSONStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	items := make([]string, 0, len(out))
	for _, item := range out {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func deriveCryptoFingerprint(seed string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(seed)))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func (h *Handlers) sessionMailPasswordFromContext(ctx context.Context) (string, error) {
	sess, ok := middleware.Session(ctx)
	if !ok {
		return "", service.ErrInvalidCredentials
	}
	return h.svc.SessionMailPassword(sess)
}

func parsePaginationV2(r *http.Request) (int, int) {
	page := 1
	pageSize := 50
	if v := strings.TrimSpace(r.URL.Query().Get("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("page_size")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			if n < 1 {
				n = 1
			}
			if n > 200 {
				n = 200
			}
			pageSize = n
		}
	}
	return page, pageSize
}

func applyPreferencesPatch(current *models.UserPreferences, patch map[string]any) {
	if v, ok := patch["theme"].(string); ok {
		current.Theme = strings.TrimSpace(v)
	}
	if v, ok := patch["density"].(string); ok {
		current.Density = strings.TrimSpace(v)
	}
	if v, ok := patch["layout_mode"].(string); ok {
		current.LayoutMode = strings.TrimSpace(v)
	}
	if v, ok := patch["remote_image_policy"].(string); ok {
		current.RemoteImagePolicy = strings.TrimSpace(v)
	}
	if v, ok := patch["timezone"].(string); ok {
		current.Timezone = strings.TrimSpace(v)
	}
	if v, ok := patch["page_size"].(float64); ok {
		current.PageSize = int(v)
	}
	if v, ok := patch["grouping_mode"].(string); ok {
		current.GroupingMode = strings.TrimSpace(v)
	}
	if v, ok := patch["keymap"]; ok {
		if b, err := json.Marshal(v); err == nil {
			current.KeymapJSON = string(b)
		}
	}
}

func mergeDraftPatch(current *models.Draft, patch map[string]any) {
	if v, ok := patch["account_id"].(string); ok {
		current.AccountID = strings.TrimSpace(v)
	}
	if v, ok := patch["identity_id"].(string); ok {
		current.IdentityID = strings.TrimSpace(v)
	}
	if v, ok := patch["compose_mode"].(string); ok {
		current.ComposeMode = strings.TrimSpace(v)
	}
	if v, ok := patch["context_message_id"].(string); ok {
		current.ContextMessageID = strings.TrimSpace(v)
	}
	if v, ok := patch["from_mode"].(string); ok {
		current.FromMode = strings.TrimSpace(v)
	}
	if v, ok := patch["from_manual"].(string); ok {
		current.FromManual = strings.TrimSpace(v)
	}
	if v, ok := patch["client_state_json"].(string); ok {
		current.ClientStateJSON = v
	}
	if v, ok := patch["to"].(string); ok {
		current.ToValue = strings.TrimSpace(v)
	}
	if v, ok := patch["cc"].(string); ok {
		current.CCValue = strings.TrimSpace(v)
	}
	if v, ok := patch["bcc"].(string); ok {
		current.BCCValue = strings.TrimSpace(v)
	}
	if v, ok := patch["subject"].(string); ok {
		current.Subject = v
	}
	if v, ok := patch["body_text"].(string); ok {
		current.BodyText = v
	}
	if v, ok := patch["body_html"].(string); ok {
		current.BodyHTML = v
	}
	if v, ok := patch["attachments_json"].(string); ok {
		current.AttachmentsJSON = v
	}
	if v, ok := patch["crypto_options_json"].(string); ok {
		current.CryptoOptions = v
	}
	if v, ok := patch["send_mode"].(string); ok {
		current.SendMode = strings.TrimSpace(v)
	}
	if v, ok := patch["status"].(string); ok {
		current.Status = strings.TrimSpace(v)
	}
	if v, ok := patch["last_send_error"].(string); ok {
		current.LastSendError = strings.TrimSpace(v)
	}
	if v, ok := patch["scheduled_for"].(string); ok {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(v)); err == nil {
			current.ScheduledFor = t.UTC()
		}
	}
}

func decodeDraftAttachmentUpload(r *http.Request) ([]models.DraftAttachment, error) {
	if err := r.ParseMultipartForm(maxUploadAttachmentBytes); err != nil {
		return nil, err
	}
	files := r.MultipartForm.File["attachments"]
	inlineFiles := r.MultipartForm.File["inline_images"]
	inlineCIDs := r.MultipartForm.Value["inline_image_cids"]
	totalBytes := int64(0)
	out := make([]models.DraftAttachment, 0, len(files)+len(inlineFiles))
	for _, fh := range files {
		if fh.Size > maxUploadAttachmentBytes {
			return nil, errors.New("attachment exceeds per-file size limit")
		}
		f, err := fh.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(f, maxUploadAttachmentBytes))
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		totalBytes += int64(len(data))
		if totalBytes > maxUploadTotalBytes {
			return nil, errors.New("attachments exceed total size limit")
		}
		out = append(out, models.DraftAttachment{
			ID:          uuid.NewString(),
			Filename:    fh.Filename,
			ContentType: fh.Header.Get("Content-Type"),
			SizeBytes:   int64(len(data)),
			Data:        data,
		})
	}
	for i, fh := range inlineFiles {
		if fh.Size > maxUploadAttachmentBytes {
			return nil, errors.New("attachment exceeds per-file size limit")
		}
		f, err := fh.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(f, maxUploadAttachmentBytes))
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		totalBytes += int64(len(data))
		if totalBytes > maxUploadTotalBytes {
			return nil, errors.New("attachments exceed total size limit")
		}
		contentID := ""
		if i < len(inlineCIDs) {
			contentID = strings.TrimSpace(inlineCIDs[i])
		}
		if contentID == "" {
			contentID = fmt.Sprintf("inline-image-%d", i+1)
		}
		out = append(out, models.DraftAttachment{
			ID:          uuid.NewString(),
			Filename:    fh.Filename,
			ContentType: fh.Header.Get("Content-Type"),
			SizeBytes:   int64(len(data)),
			InlinePart:  true,
			ContentID:   contentID,
			Data:        data,
		})
	}
	return out, nil
}

func draftAttachmentRefs(items []models.DraftAttachment) []models.DraftAttachment {
	out := make([]models.DraftAttachment, 0, len(items))
	for _, item := range items {
		out = append(out, models.DraftAttachment{
			ID:          item.ID,
			Filename:    item.Filename,
			ContentType: item.ContentType,
			SizeBytes:   item.SizeBytes,
			InlinePart:  item.InlinePart,
			ContentID:   item.ContentID,
			SortOrder:   item.SortOrder,
		})
	}
	return out
}

func (h *Handlers) syncDraftAttachmentMetadata(ctx context.Context, draft models.Draft) (models.Draft, []models.DraftAttachment, error) {
	items, err := h.svc.Store().ListDraftAttachments(ctx, draft.UserID, draft.ID)
	if err != nil {
		return models.Draft{}, nil, err
	}
	refs := draftAttachmentRefs(items)
	b, err := json.Marshal(refs)
	if err != nil {
		return models.Draft{}, nil, err
	}
	draft.AttachmentsJSON = string(b)
	if strings.EqualFold(strings.TrimSpace(draft.Status), "failed") {
		draft.Status = "active"
		draft.LastSendError = ""
	}
	updated, err := h.svc.Store().UpdateDraft(ctx, draft)
	if err != nil {
		return models.Draft{}, nil, err
	}
	return updated, refs, nil
}

func validateSieveScript(body string) error {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return fmt.Errorf("script body is required")
	}
	lower := strings.ToLower(trimmed)
	if !strings.Contains(lower, "require") {
		return fmt.Errorf("sieve script should declare require statement")
	}
	return nil
}

func generateRecoveryCodes(n int) ([]string, error) {
	if n <= 0 {
		n = 10
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		raw, err := randomToken()
		if err != nil {
			return nil, err
		}
		if len(raw) > 12 {
			raw = raw[:12]
		}
		out = append(out, strings.ToUpper(raw))
	}
	return out, nil
}

type webAuthnChallengeState struct {
	UserID    string    `json:"user_id"`
	Mode      string    `json:"mode"`
	Challenge string    `json:"challenge"`
	RPID      string    `json:"rp_id"`
	Origins   []string  `json:"origins"`
	ExpiresAt time.Time `json:"expires_at"`
}

type passkeyLoginChallengeState struct {
	Challenge            string    `json:"challenge"`
	RPID                 string    `json:"rp_id"`
	Origins              []string  `json:"origins"`
	ExpiresAt            time.Time `json:"expires_at"`
	AttemptCount         int       `json:"attempt_count"`
	MaxAttempts          int       `json:"max_attempts"`
	CookieTokenHash      string    `json:"cookie_token_hash"`
	UserID               string    `json:"user_id"`
	AllowCredentialsJSON []string  `json:"allow_credentials"`
}

type webAuthnContext struct {
	RPID          string
	Origins       []string
	RequestOrigin string
	Reason        string
}

func webAuthnLoginChallengeSettingKey(sessionID string) string {
	return "mfa:webauthn:challenge:login:" + strings.TrimSpace(sessionID)
}

func webAuthnRegisterChallengeSettingKey(userID string) string {
	return "mfa:webauthn:challenge:register:" + strings.TrimSpace(userID)
}

func passkeyLoginChallengeSettingsPrefix() string {
	return "auth:webauthn:challenge:passkey:"
}

func passkeyLoginChallengeSettingKey(challengeID string) string {
	return passkeyLoginChallengeSettingsPrefix() + strings.TrimSpace(challengeID)
}

func mfaTrustedPendingRememberSettingKey(sessionID string) string {
	return "mfa:trusted:remember:pending:" + strings.TrimSpace(sessionID)
}

func setNoStoreHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func (h *Handlers) storeWebAuthnChallenge(ctx context.Context, key string, challenge webAuthnChallengeState) error {
	b, err := json.Marshal(challenge)
	if err != nil {
		return err
	}
	return h.svc.Store().UpsertSetting(ctx, key, string(b))
}

func (h *Handlers) loadWebAuthnChallenge(ctx context.Context, key string) (webAuthnChallengeState, bool, error) {
	raw, ok, err := h.svc.Store().GetSetting(ctx, key)
	if err != nil {
		return webAuthnChallengeState{}, false, err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return webAuthnChallengeState{}, false, nil
	}
	var state webAuthnChallengeState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return webAuthnChallengeState{}, false, err
	}
	return state, true, nil
}

func (h *Handlers) storePasskeyLoginChallenge(ctx context.Context, challengeID string, challenge passkeyLoginChallengeState) error {
	b, err := json.Marshal(challenge)
	if err != nil {
		return err
	}
	return h.svc.Store().UpsertSetting(ctx, passkeyLoginChallengeSettingKey(challengeID), string(b))
}

func (h *Handlers) loadPasskeyLoginChallenge(ctx context.Context, challengeID string) (passkeyLoginChallengeState, bool, error) {
	raw, ok, err := h.svc.Store().GetSetting(ctx, passkeyLoginChallengeSettingKey(challengeID))
	if err != nil {
		return passkeyLoginChallengeState{}, false, err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return passkeyLoginChallengeState{}, false, nil
	}
	var state passkeyLoginChallengeState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return passkeyLoginChallengeState{}, false, err
	}
	return state, true, nil
}

func (h *Handlers) deletePasskeyLoginChallenge(ctx context.Context, challengeID string) error {
	return h.svc.Store().DeleteSetting(ctx, passkeyLoginChallengeSettingKey(challengeID))
}

func (h *Handlers) callMailSecOperation(ctx context.Context, op string, accountID string, payload map[string]any) (map[string]any, error) {
	if !h.mailSecRuntimeEnabled() {
		return nil, fmt.Errorf("mailsec service is disabled")
	}
	timeout := time.Duration(h.cfg.MailSecTimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cli := mailsecclient.NewClient(h.cfg.MailSecSocket)
	resp, err := cli.Call(callCtx, mailsecclient.Request{
		RequestID:  uuid.NewString(),
		Op:         op,
		AccountID:  firstNonEmpty(strings.TrimSpace(accountID), "default"),
		MessageID:  uuid.NewString(),
		Payload:    payload,
		DeadlineMS: int(timeout.Milliseconds()),
	})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("%s: %s", strings.TrimSpace(resp.Code), strings.TrimSpace(resp.Error))
	}
	if resp.Result == nil {
		return map[string]any{}, nil
	}
	return resp.Result, nil
}

func (h *Handlers) mailSecRuntimeEnabled() bool {
	if h.cfg.MailSecEnabled {
		return true
	}
	socket := strings.TrimSpace(h.cfg.MailSecSocket)
	if socket == "" {
		return false
	}
	_, err := os.Stat(socket)
	return err == nil
}

func (h *Handlers) verifyTOTPCode(ctx context.Context, userID, secret, code string) (bool, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return false, nil
	}
	if !h.cfg.MailSecEnabled {
		return totp.Validate(code, secret), nil
	}
	result, err := h.callMailSecOperation(ctx, "totp.verify", userID, map[string]any{
		"secret":      secret,
		"code":        code,
		"period":      30,
		"digits":      6,
		"algorithm":   "SHA1",
		"skew_past":   1,
		"skew_future": 1,
	})
	if err != nil {
		return false, err
	}
	valid, ok := getBoolValue(result, "valid")
	if !ok {
		return false, fmt.Errorf("mailsec response missing valid flag")
	}
	return valid, nil
}

func (h *Handlers) authCapabilities(r *http.Request) map[string]any {
	mailsecAvailable := h.mailSecRuntimeEnabled()
	context := h.resolveWebAuthnContext(r)
	mfaAvailable := mailsecAvailable && context.Reason == ""
	passkeySignInEnabled := h.cfg.PasskeyPasswordlessEnabled
	if enabled, err := h.svc.PasskeySignInEnabled(r.Context()); err == nil {
		passkeySignInEnabled = enabled
	}
	passwordlessAvailable := mfaAvailable && passkeySignInEnabled
	reason := ""
	switch {
	case !mailsecAvailable:
		reason = "mailsec_unavailable"
	case context.Reason != "":
		reason = context.Reason
	case !passkeySignInEnabled:
		reason = "passwordless_disabled"
	}
	return map[string]any{
		"passkey_mfa_available":          mfaAvailable,
		"passkey_passwordless_available": passwordlessAvailable,
		"passkey_usernameless_enabled":   true,
		"rp_id":                          context.RPID,
		"allowed_origins":                context.Origins,
		"reason":                         reason,
	}
}

func (h *Handlers) webAuthnRPID(r *http.Request) string {
	if raw := normalizeRPIDCandidate(h.cfg.WebAuthnRPID); raw != "" {
		return raw
	}
	if raw, ok, err := h.svc.Store().GetSetting(r.Context(), "base_domain"); err == nil && ok {
		if normalized := normalizeRPIDCandidate(raw); normalized != "" {
			return normalized
		}
	}
	if raw := normalizeRPIDCandidate(h.cfg.BaseDomain); raw != "" {
		return raw
	}
	return normalizeRPIDCandidate(hostWithoutPort(strings.TrimSpace(r.Host)))
}

func (h *Handlers) webAuthnAllowedOrigins(r *http.Request, requestOrigin string) []string {
	if len(h.cfg.WebAuthnAllowedOrigins) > 0 {
		out := make([]string, 0, len(h.cfg.WebAuthnAllowedOrigins))
		seen := map[string]struct{}{}
		for _, item := range h.cfg.WebAuthnAllowedOrigins {
			origin := normalizeOrigin(item)
			if origin == "" {
				continue
			}
			if _, ok := seen[origin]; ok {
				continue
			}
			seen[origin] = struct{}{}
			out = append(out, origin)
		}
		if len(out) > 0 {
			return out
		}
	}
	derived := normalizeOrigin(requestOrigin)
	if derived != "" {
		return []string{derived}
	}
	return nil
}

func (h *Handlers) requestOrigin(r *http.Request) string {
	scheme := "https"
	if h.cfg.TrustProxy {
		if xfp := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); xfp != "" {
			scheme = strings.ToLower(strings.TrimSpace(strings.Split(xfp, ",")[0]))
		} else if r.TLS == nil {
			scheme = "http"
		}
	} else if r.TLS == nil {
		scheme = "http"
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		host = h.webAuthnRPID(r)
	}
	if host == "" {
		return ""
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func (h *Handlers) resolveWebAuthnContext(r *http.Request) webAuthnContext {
	rpID := h.webAuthnRPID(r)
	requestOrigin := h.requestOrigin(r)
	origins := h.webAuthnAllowedOrigins(r, requestOrigin)
	resolved := webAuthnContext{
		RPID:          rpID,
		Origins:       origins,
		RequestOrigin: requestOrigin,
	}
	if !isWebAuthnSecureOrigin(requestOrigin) {
		resolved.Reason = "insecure_origin"
		return resolved
	}
	if strings.TrimSpace(rpID) == "" {
		resolved.Reason = "rp_mismatch"
		return resolved
	}
	if !originInList(requestOrigin, origins) {
		resolved.Reason = "origin_mismatch"
		return resolved
	}
	host := hostWithoutPort(strings.TrimSpace(r.Host))
	if host == "" {
		host = originHost(requestOrigin)
	}
	if host != "" && !isLoopbackHost(host) && !hostMatchesRPID(host, rpID) {
		resolved.Reason = "rp_mismatch"
		return resolved
	}
	return resolved
}

func (h *Handlers) requireWebAuthnContext(w http.ResponseWriter, r *http.Request) (webAuthnContext, bool) {
	resolved := h.resolveWebAuthnContext(r)
	switch resolved.Reason {
	case "":
		return resolved, true
	case "insecure_origin":
		util.WriteError(w, http.StatusBadRequest, "webauthn_insecure_origin", "webauthn requires HTTPS or localhost", middleware.RequestID(r.Context()))
	case "rp_mismatch":
		util.WriteError(w, http.StatusBadRequest, "webauthn_rp_mismatch", "request host does not match configured webauthn rp_id", middleware.RequestID(r.Context()))
	case "origin_mismatch":
		util.WriteError(w, http.StatusBadRequest, "webauthn_origin_mismatch", "request origin is not allowed for webauthn", middleware.RequestID(r.Context()))
	default:
		util.WriteError(w, http.StatusBadRequest, "webauthn_context_invalid", "webauthn context is invalid", middleware.RequestID(r.Context()))
	}
	return webAuthnContext{}, false
}

func hostWithoutPort(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if strings.HasPrefix(host, "[") && strings.Contains(host, "]") && strings.Count(host, ":") >= 2 {
		return strings.TrimPrefix(strings.SplitN(host, "]", 2)[0], "[")
	}
	if strings.Count(host, ":") == 0 {
		return strings.ToLower(strings.TrimSpace(host))
	}
	if strings.Count(host, ":") > 1 {
		return strings.TrimPrefix(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), "]"), "[")
	}
	h, _, err := net.SplitHostPort(host)
	if err == nil {
		return strings.TrimPrefix(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(h)), "]"), "[")
	}
	return strings.ToLower(strings.TrimSpace(host))
}

func normalizeRPIDCandidate(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	value = strings.TrimSuffix(value, "/")
	value = strings.TrimSuffix(value, ".")
	return hostWithoutPort(value)
}

func normalizeOrigin(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Scheme)) + "://" + strings.ToLower(strings.TrimSpace(parsed.Host))
}

func originHost(origin string) string {
	parsed, err := url.Parse(strings.TrimSpace(origin))
	if err != nil {
		return ""
	}
	return hostWithoutPort(parsed.Host)
}

func originInList(origin string, allowed []string) bool {
	normalizedOrigin := normalizeOrigin(origin)
	if normalizedOrigin == "" {
		return false
	}
	for _, item := range allowed {
		if subtleConstantCompare(normalizeOrigin(item), normalizedOrigin) == 1 {
			return true
		}
	}
	return false
}

func isWebAuthnSecureOrigin(origin string) bool {
	parsed, err := url.Parse(strings.TrimSpace(origin))
	if err != nil {
		return false
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	host := hostWithoutPort(parsed.Host)
	if scheme == "https" {
		return true
	}
	if scheme != "http" {
		return false
	}
	return isLoopbackHost(host)
}

func isLoopbackHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func hostMatchesRPID(host, rpID string) bool {
	h := normalizeRPIDCandidate(host)
	rp := normalizeRPIDCandidate(rpID)
	if h == "" || rp == "" {
		return false
	}
	if subtleConstantCompare(h, rp) == 1 {
		return true
	}
	return strings.HasSuffix(h, "."+rp)
}

func passkeyLoginChallengeCookieName() string {
	return "despatch_passkey_challenge"
}

func (h *Handlers) setPasskeyLoginChallengeCookie(w http.ResponseWriter, r *http.Request, challengeID, nonce string, expiresAt time.Time) {
	secure := h.cfg.ResolveCookieSecure(r)
	value := strings.TrimSpace(challengeID) + "." + strings.TrimSpace(nonce)
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	http.SetCookie(w, &http.Cookie{
		Name:     passkeyLoginChallengeCookieName(),
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
		Expires:  expiresAt,
	})
}

func (h *Handlers) clearPasskeyLoginChallengeCookie(w http.ResponseWriter, r *http.Request) {
	secure := h.cfg.ResolveCookieSecure(r)
	expiredAt := time.Unix(1, 0).UTC()
	http.SetCookie(w, &http.Cookie{
		Name:     passkeyLoginChallengeCookieName(),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  expiredAt,
	})
}

func (h *Handlers) readPasskeyLoginChallengeCookie(r *http.Request) (string, string, bool) {
	cookie, err := r.Cookie(passkeyLoginChallengeCookieName())
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimSpace(cookie.Value), ".", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	challengeID := strings.TrimSpace(parts[0])
	nonce := strings.TrimSpace(parts[1])
	if challengeID == "" || nonce == "" {
		return "", "", false
	}
	return challengeID, nonce, true
}

func summarizeTrustedDeviceLabel(raw string) (displayLabel, browser, osLabel, deviceType string) {
	label := strings.TrimSpace(raw)
	lower := strings.ToLower(label)

	browser = detectTrustedDeviceBrowser(lower)
	osLabel = detectTrustedDeviceOS(lower)
	deviceType = detectTrustedDeviceType(lower)

	parts := make([]string, 0, 3)
	if browser != "Unknown browser" {
		parts = append(parts, browser)
	}
	if osLabel != "Unknown OS" {
		parts = append(parts, osLabel)
	}
	if deviceType != "Unknown device" {
		parts = append(parts, deviceType)
	}
	if len(parts) > 0 {
		return strings.Join(parts, " / "), browser, osLabel, deviceType
	}
	if label == "" {
		return "Trusted device", browser, osLabel, deviceType
	}
	if len(label) > 96 {
		label = label[:96] + "..."
	}
	return label, browser, osLabel, deviceType
}

func detectTrustedDeviceBrowser(lowerUA string) string {
	switch {
	case strings.Contains(lowerUA, "edg/"), strings.Contains(lowerUA, "edge/"):
		return "Microsoft Edge"
	case strings.Contains(lowerUA, "opr/"), strings.Contains(lowerUA, "opera"):
		return "Opera"
	case strings.Contains(lowerUA, "samsungbrowser/"):
		return "Samsung Internet"
	case strings.Contains(lowerUA, "crios/"), strings.Contains(lowerUA, "chrome/"):
		return "Google Chrome"
	case strings.Contains(lowerUA, "fxios/"), strings.Contains(lowerUA, "firefox/"):
		return "Mozilla Firefox"
	case strings.Contains(lowerUA, "safari/") && !strings.Contains(lowerUA, "chrome/") && !strings.Contains(lowerUA, "crios/"):
		return "Safari"
	default:
		return "Unknown browser"
	}
}

func detectTrustedDeviceOS(lowerUA string) string {
	switch {
	case strings.Contains(lowerUA, "windows nt"):
		return "Windows"
	case strings.Contains(lowerUA, "iphone"), strings.Contains(lowerUA, "ipad"), strings.Contains(lowerUA, "ios"):
		return "iOS"
	case strings.Contains(lowerUA, "mac os"), strings.Contains(lowerUA, "macintosh"):
		return "macOS"
	case strings.Contains(lowerUA, "android"):
		return "Android"
	case strings.Contains(lowerUA, "ubuntu"):
		return "Ubuntu"
	case strings.Contains(lowerUA, "linux"):
		return "Linux"
	case strings.Contains(lowerUA, "cros"):
		return "ChromeOS"
	default:
		return "Unknown OS"
	}
}

func detectTrustedDeviceType(lowerUA string) string {
	switch {
	case strings.Contains(lowerUA, "ipad"), strings.Contains(lowerUA, "tablet"):
		return "Tablet"
	case strings.Contains(lowerUA, "iphone"), strings.Contains(lowerUA, "android"), strings.Contains(lowerUA, "mobile"):
		return "Mobile"
	case strings.Contains(lowerUA, "windows"), strings.Contains(lowerUA, "macintosh"), strings.Contains(lowerUA, "linux"), strings.Contains(lowerUA, "ubuntu"), strings.Contains(lowerUA, "x11"):
		return "Desktop"
	default:
		return "Unknown device"
	}
}

func (h *Handlers) findMFAWebAuthnCredentialAnyUserByAnyID(ctx context.Context, credentialID string) (models.MFAWebAuthnCredential, error) {
	for _, candidate := range webAuthnCredentialIDCandidates(credentialID) {
		item, err := h.svc.Store().GetMFAWebAuthnCredentialByCredentialIDAnyUser(ctx, candidate)
		if err == nil {
			return item, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return models.MFAWebAuthnCredential{}, err
		}
	}
	return models.MFAWebAuthnCredential{}, store.ErrNotFound
}

func (h *Handlers) findMFAWebAuthnCredentialByAnyID(ctx context.Context, userID, credentialID string) (models.MFAWebAuthnCredential, error) {
	for _, candidate := range webAuthnCredentialIDCandidates(credentialID) {
		item, err := h.svc.Store().GetMFAWebAuthnCredentialByCredentialID(ctx, userID, candidate)
		if err == nil {
			return item, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return models.MFAWebAuthnCredential{}, err
		}
	}
	return models.MFAWebAuthnCredential{}, store.ErrNotFound
}

func getMapValue(m map[string]any, key string) map[string]any {
	v, ok := m[key]
	if !ok {
		return map[string]any{}
	}
	out, ok := v.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return out
}

func getStringValue(m map[string]any, keys ...string) string {
	for _, key := range keys {
		v, ok := m[key]
		if !ok {
			continue
		}
		if s, ok := v.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func getStringSliceValue(m map[string]any, key string) []string {
	v, ok := m[key]
	if !ok {
		return nil
	}
	switch vv := v.(type) {
	case []string:
		out := make([]string, 0, len(vv))
		for _, item := range vv {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(vv))
		for _, raw := range vv {
			s, ok := raw.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func getInt64Value(m map[string]any, key string) (int64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch vv := v.(type) {
	case int:
		return int64(vv), true
	case int64:
		return vv, true
	case float64:
		return int64(vv), true
	case json.Number:
		n, err := vv.Int64()
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func getBoolValue(m map[string]any, key string) (bool, bool) {
	v, ok := m[key]
	if !ok {
		return false, false
	}
	switch vv := v.(type) {
	case bool:
		return vv, true
	case string:
		switch strings.ToLower(strings.TrimSpace(vv)) {
		case "true":
			return true, true
		case "false":
			return false, true
		default:
			return false, false
		}
	default:
		return false, false
	}
}

func canonicalWebAuthnCredentialID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	decoded, ok := decodeWebAuthnCredentialID(trimmed)
	if !ok {
		return trimmed
	}
	return base64.RawURLEncoding.EncodeToString(decoded)
}

func webAuthnCredentialIDCandidates(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	add(trimmed)
	decoded, ok := decodeWebAuthnCredentialID(trimmed)
	if !ok {
		return out
	}
	add(base64.RawURLEncoding.EncodeToString(decoded))
	hexLower := hex.EncodeToString(decoded)
	add(hexLower)
	add(strings.ToUpper(hexLower))
	return out
}

func webAuthnCredentialIDEqual(left, right string) bool {
	leftTrimmed := strings.TrimSpace(left)
	rightTrimmed := strings.TrimSpace(right)
	if leftTrimmed == "" || rightTrimmed == "" {
		return false
	}
	leftDecoded, leftOK := decodeWebAuthnCredentialID(leftTrimmed)
	rightDecoded, rightOK := decodeWebAuthnCredentialID(rightTrimmed)
	if leftOK && rightOK {
		if len(leftDecoded) != len(rightDecoded) {
			return false
		}
		return subtle.ConstantTimeCompare(leftDecoded, rightDecoded) == 1
	}
	return subtleConstantCompare(leftTrimmed, rightTrimmed) == 1
}

func decodeWebAuthnCredentialID(raw string) ([]byte, bool) {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return nil, false
	}
	if isHexCredentialID(cleaned) {
		if out, err := hex.DecodeString(cleaned); err == nil && len(out) > 0 {
			return out, true
		}
	}
	decoders := []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	}
	for _, dec := range decoders {
		out, err := dec.DecodeString(cleaned)
		if err != nil || len(out) == 0 {
			continue
		}
		return out, true
	}
	return nil, false
}

func isHexCredentialID(raw string) bool {
	if len(raw) == 0 || len(raw)%2 != 0 {
		return false
	}
	for _, ch := range raw {
		switch {
		case ch >= '0' && ch <= '9':
		case ch >= 'a' && ch <= 'f':
		case ch >= 'A' && ch <= 'F':
		default:
			return false
		}
	}
	return true
}

func firstNonEmptySlice(values ...[]string) []string {
	for _, v := range values {
		if len(v) == 0 {
			continue
		}
		out := make([]string, 0, len(v))
		for _, raw := range v {
			item := strings.TrimSpace(raw)
			if item != "" {
				out = append(out, item)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func parseWebAuthnTransportsJSON(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{}
	}
	return out
}

func subtleConstantCompare(a, b string) int {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		s := strings.TrimSpace(v)
		if s != "" {
			return s
		}
	}
	return ""
}

func firstPositive(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}
