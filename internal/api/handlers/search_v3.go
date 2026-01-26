package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-busca-search/internal/models/v3"
	"github.com/prefeitura-rio/app-busca-search/internal/search"
)

// SearchHandlerV3 gerencia endpoints de busca v3
type SearchHandlerV3 struct {
	engine *search.Engine
}

// NewSearchHandlerV3 cria um novo handler de busca v3
func NewSearchHandlerV3(engine *search.Engine) *SearchHandlerV3 {
	return &SearchHandlerV3{
		engine: engine,
	}
}

// Search godoc
// @Summary Busca unificada v3
// @Description ## Busca Unificada v3
// @Description
// @Description Endpoint principal de busca com suporte a múltiplos algoritmos e modos de resposta.
// @Description
// @Description ### Tipos de Busca (parâmetro `type`)
// @Description - **keyword**: Busca textual BM25 pura. Ideal para termos exatos e nomes de serviços.
// @Description - **semantic**: Busca vetorial usando embeddings Gemini. Ideal para queries em linguagem natural.
// @Description - **hybrid**: Combina keyword + semantic com peso configurável via `alpha`. Recomendado para uso geral.
// @Description - **ai**: Busca inteligente com análise LLM da query, expansão automática e reranking. Maior latência.
// @Description
// @Description ### Modos de Resposta (parâmetro `mode`)
// @Description - **human** (padrão): Resposta completa com todos os metadados, scores detalhados e timing.
// @Description - **agent**: Resposta compacta otimizada para chatbots/agentes IA. Inclui apenas campos essenciais e botões de ação.
// @Description
// @Description ### Fórmula do Score Híbrido
// @Description ```
// @Description hybrid_score = (alpha × text_score) + ((1 - alpha) × vector_score)
// @Description ```
// @Description
// @Description ### Comportamento por Modo
// @Description | Parâmetro | mode=human | mode=agent |
// @Description |-----------|------------|------------|
// @Description | expand    | true       | false      |
// @Description | recency   | true       | false      |
// @Description | typos     | 2          | 1          |
// @Description
// @Description ### Tipos de Resposta (200 OK)
// @Description O formato da resposta varia conforme os parâmetros:
// @Description
// @Description 1. **SearchResponse** (padrão): Quando `mode=human` e sem `fields`. Inclui scores detalhados, timing e metadados completos.
// @Description 2. **AgentSearchResponse**: Quando `mode=agent`. Formato compacto com apenas campos essenciais e botões de ação.
// @Description 3. **FilteredSearchResponse**: Quando `fields` é especificado. Inclui apenas os campos solicitados para reduzir payload.
// @Description
// @Description Consulte os schemas `v3.SearchResponse`, `v3.AgentSearchResponse` e `v3.FilteredSearchResponse` para detalhes.
// @Description
// @Tags search-v3
// @Accept json
// @Produce json
// @Param q query string true "Query de busca. Termos a serem pesquisados." example("certidão de nascimento")
// @Param type query string true "Tipo de busca a ser executada" Enums(keyword, semantic, hybrid, ai) example("hybrid")
// @Param page query int false "Página de resultados (começa em 1)" default(1) minimum(1) example(1)
// @Param per_page query int false "Quantidade de resultados por página" default(10) minimum(1) maximum(100) example(10)
// @Param collections query string false "Collections para buscar (comma-separated). Se vazio, busca em todas." example("prefrio_services_base,1746")
// @Param mode query string false "Modo de resposta. 'agent' retorna formato compacto para chatbots." Enums(human, agent) default(human) example("human")
// @Param alpha query number false "Peso do score textual vs vetorial (0.0 a 1.0). Alpha=1.0 é 100% texto, Alpha=0.0 é 100% vetor." default(0.5) minimum(0) maximum(1) example(0.5)
// @Param threshold query number false "Score mínimo para incluir resultado (0.0 a 1.0). Resultados abaixo são filtrados." minimum(0) maximum(1) example(0.3)
// @Param expand query bool false "Expandir query com sinônimos. Default: true para human, false para agent." example(true)
// @Param recency query bool false "Aplicar boost de recência (documentos recentes pontuam mais). Default: true para human, false para agent." example(true)
// @Param typos query int false "Tolerância a erros de digitação (0=exato, 1=pouco tolerante, 2=muito tolerante). Default: 2 para human, 1 para agent." minimum(0) maximum(2) example(2)
// @Param status query int false "Filtrar por status do serviço. 0=Rascunho, 1=Publicado." Enums(0, 1) example(1)
// @Param category query string false "Filtrar por categoria (tema_geral). Case-insensitive." example("documentos")
// @Param sub_category query string false "Filtrar por subcategoria. Case-insensitive." example("certidoes")
// @Param orgao query string false "Filtrar por órgão gestor responsável pelo serviço." example("SMDC")
// @Param tempo_max query string false "Filtrar por tempo máximo de atendimento." Enums(imediato, 1_dia, 2_a_5_dias, 6_a_10_dias, 11_a_15_dias, 16_a_30_dias, mais_de_30_dias) example("imediato")
// @Param is_free query bool false "Filtrar apenas serviços gratuitos (true) ou pagos (false)." example(true)
// @Param digital query bool false "Filtrar serviços com canal digital disponível." example(true)
// @Param fields query string false "Campos específicos a retornar (comma-separated). Reduz payload. Campos: id,collection,type,title,description,category,slug,url,score,data,buttons" example("title,description,score,buttons")
// @Success 200 {object} v3.SearchResponse "Resposta completa (mode=human, sem fields)"
// @Success 200 {object} v3.AgentSearchResponse "Resposta compacta para agentes (mode=agent)"
// @Success 200 {object} v3.FilteredSearchResponse "Resposta com campos filtrados (quando fields é especificado)"
// @Failure 400 {object} map[string]string "Parâmetros inválidos ou query vazia"
// @Failure 500 {object} map[string]string "Erro interno do servidor"
// @Router /api/v3/search [get]
func (h *SearchHandlerV3) Search(c *gin.Context) {
	var req v3.SearchRequest

	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Parâmetros inválidos",
			"details": err.Error(),
		})
		return
	}

	// Parse collections
	if req.Collections != "" {
		req.ParsedCollections = parseCollections(req.Collections)
	}

	// Parse fields
	req.ParseFields()

	// Executa busca
	result, err := h.engine.Search(c.Request.Context(), &req)
	if err != nil {
		status := http.StatusInternalServerError
		if err == v3.ErrQueryRequired || err == v3.ErrInvalidSearchType {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	// Retorna resposta compacta para modo agent
	if req.Mode == v3.SearchModeAgent {
		c.JSON(http.StatusOK, result.ToAgentResponse())
		return
	}

	// Retorna campos filtrados se especificado
	if len(req.ParsedFields) > 0 {
		c.JSON(http.StatusOK, result.ToFilteredResponse(req.ParsedFields))
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetDocument godoc
// @Summary Busca documento por ID
// @Description Retorna um documento específico pelo seu identificador único.
// @Description
// @Description Se o parâmetro `collection` for informado, a busca é mais eficiente pois vai direto na collection especificada.
// @Description Caso contrário, o sistema busca em todas as collections disponíveis.
// @Tags search-v3
// @Accept json
// @Produce json
// @Param id path string true "Identificador único do documento" example("srv_12345")
// @Param collection query string false "Nome da collection para otimizar a busca. Se omitido, busca em todas." example("prefrio_services_base")
// @Success 200 {object} v3.Document "Documento encontrado com todos os campos"
// @Failure 400 {object} map[string]string "ID não informado"
// @Failure 404 {object} map[string]string "Documento não encontrado"
// @Router /api/v3/search/{id} [get]
func (h *SearchHandlerV3) GetDocument(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID e obrigatorio"})
		return
	}

	// Collection hint opcional para otimizar a busca
	collection := c.Query("collection")

	doc, err := h.engine.GetDocument(c.Request.Context(), id, collection)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Documento nao encontrado"})
		return
	}

	c.JSON(http.StatusOK, doc)
}

func parseCollections(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
