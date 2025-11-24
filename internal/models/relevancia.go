package models

// RelevanciaItem representa um item com sua relevância baseada em acessos
type RelevanciaItem struct {
	Titulo     string `json:"titulo"`
	Acessos    int    `json:"acessos"`
	Fonte      string `json:"fonte"`      // "1746" ou "carioca-digital"
	Relevancia int    `json:"relevancia"` // Score calculado baseado nos acessos
}

// RelevanciaService armazena dados de relevância para ordenação
type RelevanciaData struct {
	ItensRelevancia   map[string]*RelevanciaItem `json:"itens_relevancia"` // Chave: título normalizado
	UltimaAtualizacao string                     `json:"ultima_atualizacao"`
}

// RelevanciaConfig contém configurações para carregamento de dados de relevância
type RelevanciaConfig struct {
	CaminhoArquivo1746           string `json:"caminho_arquivo_1746"`
	CaminhoArquivoCariocaDigital string `json:"caminho_arquivo_carioca_digital"`
	IntervaloAtualizacao         int    `json:"intervalo_atualizacao"` // em minutos
}
