package utils

import (
	"fmt"
	"net/url"
	"strings"
)

// TargetDomains são os domínios que devem ser encapsulados pelo gateway
var TargetDomains = []string{
	"services-carioca.rio.rj.gov.br",
	"acesso.processo.rio",
}

// WrapURLIfNeeded verifica se a URL aponta para um dos domínios-alvo
// e a encapsula no gateway se necessário
func WrapURLIfNeeded(originalURL, gatewayBaseURL string) string {
	// Se o gateway não estiver configurado, retorna a URL original
	if gatewayBaseURL == "" {
		return originalURL
	}

	// Se a URL estiver vazia, retorna vazia
	if strings.TrimSpace(originalURL) == "" {
		return originalURL
	}

	// Parse da URL original
	parsedURL, err := url.Parse(originalURL)
	if err != nil {
		// Se não conseguir parsear, retorna original
		return originalURL
	}

	// Verifica se o host é um dos domínios-alvo
	isTargetDomain := false
	for _, domain := range TargetDomains {
		if strings.Contains(parsedURL.Host, domain) {
			isTargetDomain = true
			break
		}
	}

	// Se não for domínio-alvo, retorna original
	if !isTargetDomain {
		return originalURL
	}

	// Se já estiver encapsulada no gateway, retorna original
	if strings.Contains(originalURL, gatewayBaseURL) {
		return originalURL
	}

	// Encapsula no gateway
	encodedURL := url.QueryEscape(originalURL)
	wrappedURL := fmt.Sprintf("%s/gateway?urlServico=%s", gatewayBaseURL, encodedURL)

	return wrappedURL
}

// WrapURLsInArray aplica WrapURLIfNeeded para cada URL em um array
func WrapURLsInArray(urls []string, gatewayBaseURL string) []string {
	if len(urls) == 0 {
		return urls
	}

	wrapped := make([]string, len(urls))
	for i, u := range urls {
		wrapped[i] = WrapURLIfNeeded(u, gatewayBaseURL)
	}
	return wrapped
}
