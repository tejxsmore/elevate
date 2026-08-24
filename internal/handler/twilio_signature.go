package handler

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

func validateTwilioSignature(
	r *http.Request,
	authToken string,
) bool {
	signature := strings.TrimSpace(
		r.Header.Get("X-Twilio-Signature"),
	)

	if signature == "" ||
		strings.TrimSpace(authToken) == "" {
		return false
	}

	publicURL := requestPublicURL(r)

	values, err := url.ParseQuery(
		r.URL.RawQuery,
	)
	if err != nil {
		return false
	}

	formValues := r.PostForm

	if formValues == nil {
		if err := r.ParseForm(); err != nil {
			return false
		}

		formValues = r.PostForm
	}

	keys := make(
		[]string,
		0,
		len(values)+len(formValues),
	)

	seen := make(
		map[string]struct{},
	)

	for key := range values {
		if _, ok := seen[key]; ok {
			continue
		}

		seen[key] = struct{}{}
		keys = append(keys, key)
	}

	for key := range formValues {
		if _, ok := seen[key]; ok {
			continue
		}

		seen[key] = struct{}{}
		keys = append(keys, key)
	}

	sort.Strings(keys)

	message := publicURL

	for _, key := range keys {
		for _, value := range formValues[key] {
			message += key + value
		}
	}

	mac := hmac.New(
		sha1.New,
		[]byte(authToken),
	)

	_, _ = mac.Write(
		[]byte(message),
	)

	expected := base64.StdEncoding.EncodeToString(
		mac.Sum(nil),
	)

	return hmac.Equal(
		[]byte(expected),
		[]byte(signature),
	)
}

func requestPublicURL(
	r *http.Request,
) string {
	scheme := strings.TrimSpace(
		r.Header.Get("X-Forwarded-Proto"),
	)

	if scheme == "" {
		scheme = "https"
	}

	host := strings.TrimSpace(
		r.Header.Get("X-Forwarded-Host"),
	)

	if host == "" {
		host = r.Host
	}

	if host == "" {
		host = r.URL.Host
	}

	return scheme + "://" + host + r.URL.RequestURI()
}
