package mail

import (
	"fmt"
	"io"
	"mime"
	stdmail "net/mail"
	"strings"

	"golang.org/x/net/html/charset"
)

var headerWordDecoder = mime.WordDecoder{
	CharsetReader: func(name string, input io.Reader) (io.Reader, error) {
		return charset.NewReaderLabel(name, input)
	},
}

func DecodeHeaderText(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	decoded, err := headerWordDecoder.DecodeHeader(value)
	if err != nil {
		return value
	}
	return strings.TrimSpace(decoded)
}

func FormatDisplayAddress(name, address string) string {
	decodedName := DecodeHeaderText(name)
	trimmedAddress := strings.TrimSpace(address)
	switch {
	case decodedName == "" && trimmedAddress == "":
		return ""
	case decodedName == "":
		return trimmedAddress
	case trimmedAddress == "":
		return decodedName
	default:
		return fmt.Sprintf("%s <%s>", decodedName, trimmedAddress)
	}
}

func FormatAddress(addr *stdmail.Address) string {
	if addr == nil {
		return ""
	}
	return FormatDisplayAddress(addr.Name, addr.Address)
}

func FormatAddressList(in []*stdmail.Address) []string {
	out := make([]string, 0, len(in))
	for _, addr := range in {
		if value := strings.TrimSpace(FormatAddress(addr)); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func DecodeAddressListValue(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if parsed, err := stdmail.ParseAddressList(value); err == nil && len(parsed) > 0 {
		return strings.Join(FormatAddressList(parsed), ", ")
	}
	if parsed, err := stdmail.ParseAddress(value); err == nil {
		return FormatAddress(parsed)
	}
	return DecodeHeaderText(value)
}
