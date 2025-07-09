package models

// Documento representa um documento completo baseado no novo schema do Typesense
type Documento struct {
	ID                      string  `json:"id,omitempty"`
	Titulo                  string  `json:"titulo"`
	Descricao               string  `json:"descricao"`
	URL                     string  `json:"url"`
	Collection              string  `json:"collection"`
	Tipo                    string  `json:"tipo"`
	Category                string  `json:"category"`
	UltimaAtualizacao       string  `json:"ultima_atualizacao"`
	InformacoesComplementares *string `json:"informacoes_complementares,omitempty"`
	Etapas                  *string `json:"etapas,omitempty"`
	PrazoEsperado           *string `json:"prazo_esperado,omitempty"`
	OrgaoGestor             *string `json:"orgao_gestor,omitempty"`
	CustoDoServico          *string `json:"custo_do_servico,omitempty"`
	LinkAcesso              *string `json:"link_acesso,omitempty"`
	IDCariocaDigital        *string `json:"id_carioca_digital,omitempty"`
	ID1746                  *string `json:"id_1746,omitempty"`
	IDPrefRio               *string `json:"id_pref_rio,omitempty"`
}

// DocumentoResumo representa um documento com apenas título e ID para busca por categoria
type DocumentoResumo struct {
	ID     string `json:"id"`
	Titulo string `json:"titulo"`
}

// BuscaCategoriaResponse representa a resposta da busca por categoria
type BuscaCategoriaResponse struct {
	Found     int               `json:"found"`
	OutOf     int               `json:"out_of"`
	Page      int               `json:"page"`
	Documentos []DocumentoResumo `json:"documentos"`
} 