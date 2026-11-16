package models

import "testing"

func TestMaskCPF(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "só dígitos", in: "12345678909", want: "***.456.789-**"},
		{name: "com pontuação", in: "123.456.789-09", want: "***.456.789-**"},
		{name: "com espaços nas bordas", in: " 123.456.789-09 ", want: "***.456.789-**"},
		{name: "vazio continua vazio", in: "", want: ""},
		{name: "só espaços continua vazio", in: "   ", want: ""},
		{name: "dígitos de menos é ocultado", in: "1234567", want: "***"},
		{name: "dígitos de mais é ocultado", in: "123456789012", want: "***"},
		{name: "texto que não é CPF é ocultado", in: "fulano@example.com", want: "***"},
		{name: "já mascarado continua mascarado", in: "***.456.789-**", want: "***"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskCPF(tt.in); got != tt.want {
				t.Errorf("MaskCPF(%q) = %q, esperado %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestServiceVersionWithMaskedCPFDoesNotChangeOriginal(t *testing.T) {
	original := ServiceVersion{ServiceID: "svc-1", CreatedBy: "Autor Teste", CreatedByCPF: "12345678909"}

	masked := original.WithMaskedCPF()

	if masked.CreatedByCPF != "***.456.789-**" {
		t.Errorf("CreatedByCPF mascarado = %q", masked.CreatedByCPF)
	}
	if masked.CreatedBy != "Autor Teste" || masked.ServiceID != "svc-1" {
		t.Errorf("outros campos mudaram: %+v", masked)
	}
	if original.CreatedByCPF != "12345678909" {
		t.Errorf("original alterado: %q", original.CreatedByCPF)
	}
}
