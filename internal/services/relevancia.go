package services

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prefeitura-rio/app-busca-search/internal/models"
)

type RelevanciaService struct {
	data           *models.RelevanciaData
	config         *models.RelevanciaConfig
	mutex          sync.RWMutex
	ultimaAtualizacao time.Time
}

func NewRelevanciaService(config *models.RelevanciaConfig) *RelevanciaService {
	service := &RelevanciaService{
		data: &models.RelevanciaData{
			ItensRelevancia: make(map[string]*models.RelevanciaItem),
		},
		config: config,
	}
	
	// Carrega dados iniciais
	service.CarregarDados()
	
	// Inicia rotina para atualização periódica
	go service.iniciarAtualizacaoAutomatica()
	
	return service
}

// CarregarDados carrega dados de relevância das fontes configuradas
func (s *RelevanciaService) CarregarDados() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	log.Println("Carregando dados de relevância...")
	
	// Log do diretório atual para debugging
	pwd, _ := os.Getwd()
	log.Printf("Diretório atual: %s", pwd)
	
	// Limpa dados existentes
	s.data.ItensRelevancia = make(map[string]*models.RelevanciaItem)
	
	// Carrega dados do 1746 (planilha)
	if s.config.CaminhoArquivo1746 != "" {
		log.Printf("Tentando carregar dados do 1746 de: %s", s.config.CaminhoArquivo1746)
		if err := s.carregarDados1746(); err != nil {
			log.Printf("Erro ao carregar dados do 1746: %v", err)
		} else {
			log.Println("Dados do 1746 carregados com sucesso")
		}
	}
	
	// Carrega dados do carioca-digital (CSV)
	if s.config.CaminhoArquivoCariocaDigital != "" {
		log.Printf("Tentando carregar dados do carioca-digital de: %s", s.config.CaminhoArquivoCariocaDigital)
		if err := s.carregarDadosCariocaDigital(); err != nil {
			log.Printf("Erro ao carregar dados do carioca-digital: %v", err)
		} else {
			log.Println("Dados do carioca-digital carregados com sucesso")
		}
	}
	
	// Calcula relevância baseada nos acessos
	s.calcularRelevancia()
	
	s.data.UltimaAtualizacao = time.Now().Format(time.RFC3339)
	s.ultimaAtualizacao = time.Now()
	
	log.Printf("Dados de relevância carregados: %d itens", len(s.data.ItensRelevancia))
	
	return nil
}

// carregarDados1746 carrega dados de volumetria do 1746 de uma planilha CSV
func (s *RelevanciaService) carregarDados1746() error {
	// Resolve o caminho do arquivo
	resolvedPath, err := s.resolvePath(s.config.CaminhoArquivo1746)
	if err != nil {
		return fmt.Errorf("erro ao resolver caminho do arquivo 1746: %v", err)
	}
	
	log.Printf("Abrindo arquivo 1746: %s", resolvedPath)
	
	file, err := os.Open(resolvedPath)
	if err != nil {
		return fmt.Errorf("erro ao abrir arquivo do 1746: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = ',' // Usando vírgula como separador
	
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("erro ao ler CSV do 1746: %v", err)
	}

	// Procura pelos índices das colunas corretas
	var nomeServicoIdx, totalGeralIdx int = -1, -1
	
	if len(records) > 0 {
		header := records[0]
		for i, col := range header {
			switch strings.TrimSpace(col) {
			case "Nome do serviço":
				nomeServicoIdx = i
			case "Total Geral":
				totalGeralIdx = i
			}
		}
	}
	
	if nomeServicoIdx == -1 || totalGeralIdx == -1 {
		return fmt.Errorf("colunas 'Nome do serviço' ou 'Total Geral' não encontradas no CSV do 1746")
	}

	// Processa os dados (ignora o cabeçalho)
	for i, record := range records {
		if i == 0 {
			continue
		}
		
		if len(record) <= nomeServicoIdx || len(record) <= totalGeralIdx {
			continue
		}
		
		titulo := strings.TrimSpace(record[nomeServicoIdx])
		acessosStr := strings.TrimSpace(record[totalGeralIdx])
		
		// Ignora linhas vazias
		if titulo == "" || acessosStr == "" {
			continue
		}
		
		acessos, err := strconv.Atoi(acessosStr)
		if err != nil {
			log.Printf("Erro ao converter acessos para int: %v, valor: %s", err, acessosStr)
			continue
		}
		
		tituloNorm := s.normalizarTitulo(titulo)
		
		item := &models.RelevanciaItem{
			Titulo:  titulo,
			Acessos: acessos,
			Fonte:   "1746",
		}
		
		s.data.ItensRelevancia[tituloNorm] = item
	}
	
	return nil
}

// carregarDadosCariocaDigital carrega dados de volumetria do carioca-digital
func (s *RelevanciaService) carregarDadosCariocaDigital() error {
	// Resolve o caminho do arquivo
	resolvedPath, err := s.resolvePath(s.config.CaminhoArquivoCariocaDigital)
	if err != nil {
		return fmt.Errorf("erro ao resolver caminho do arquivo carioca-digital: %v", err)
	}
	
	log.Printf("Abrindo arquivo carioca-digital: %s", resolvedPath)
	
	file, err := os.Open(resolvedPath)
	if err != nil {
		return fmt.Errorf("erro ao abrir arquivo do carioca-digital: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = ',' // Usando vírgula como separador
	
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("erro ao ler CSV do carioca-digital: %v", err)
	}

	// Procura pelos índices das colunas corretas
	var nomeServicoIdx, totalGeralIdx int = -1, -1
	
	if len(records) > 0 {
		header := records[0]
		for i, col := range header {
			switch strings.TrimSpace(col) {
			case "Nome do serviço":
				nomeServicoIdx = i
			case "Total Geral":
				totalGeralIdx = i
			}
		}
	}
	
	if nomeServicoIdx == -1 || totalGeralIdx == -1 {
		return fmt.Errorf("colunas 'Nome do serviço' ou 'Total Geral' não encontradas no CSV do carioca-digital")
	}

	// Processa os dados (ignora o cabeçalho)
	for i, record := range records {
		if i == 0 {
			continue
		}
		
		if len(record) <= nomeServicoIdx || len(record) <= totalGeralIdx {
			continue
		}
		
		titulo := strings.TrimSpace(record[nomeServicoIdx])
		acessosStr := strings.TrimSpace(record[totalGeralIdx])
		
		// Ignora linhas vazias
		if titulo == "" || acessosStr == "" {
			continue
		}
		
		acessos, err := strconv.Atoi(acessosStr)
		if err != nil {
			log.Printf("Erro ao converter acessos para int: %v, valor: %s", err, acessosStr)
			continue
		}
		
		tituloNorm := s.normalizarTitulo(titulo)
		
		// Se já existe item do 1746, soma os acessos
		if existente, exists := s.data.ItensRelevancia[tituloNorm]; exists {
			existente.Acessos += acessos
			existente.Fonte = "ambos"
		} else {
			item := &models.RelevanciaItem{
				Titulo:  titulo,
				Acessos: acessos,
				Fonte:   "carioca-digital",
			}
			s.data.ItensRelevancia[tituloNorm] = item
		}
	}
	
	return nil
}

// calcularRelevancia calcula o score de relevância baseado nos acessos
func (s *RelevanciaService) calcularRelevancia() {
	if len(s.data.ItensRelevancia) == 0 {
		return
	}
	
	// Coleta todos os acessos para calcular percentis
	var acessos []int
	for _, item := range s.data.ItensRelevancia {
		acessos = append(acessos, item.Acessos)
	}
	
	sort.Ints(acessos)
	
	// Calcula relevância baseada em percentis
	for _, item := range s.data.ItensRelevancia {
		item.Relevancia = s.calcularScoreRelevancia(item.Acessos, acessos)
	}
}

// calcularScoreRelevancia calcula o score de relevância de um item baseado nos acessos
func (s *RelevanciaService) calcularScoreRelevancia(acessos int, todosAcessos []int) int {
	if len(todosAcessos) == 0 {
		return 0
	}
	
	// Encontra posição do item na lista ordenada
	posicao := 0
	for i, acesso := range todosAcessos {
		if acesso <= acessos {
			posicao = i
		}
	}
	
	// Converte para score de 0 a 100
	percentil := float64(posicao) / float64(len(todosAcessos))
	return int(percentil * 100)
}

// normalizarTitulo normaliza o título para uso como chave
func (s *RelevanciaService) normalizarTitulo(titulo string) string {
	// Remove espaços extras, converte para minúsculas e remove acentos
	titulo = strings.ToLower(strings.TrimSpace(titulo))
	
	// Substituições básicas para normalização
	replacements := map[string]string{
		"á": "a", "à": "a", "ã": "a", "â": "a",
		"é": "e", "ê": "e",
		"í": "i", "î": "i",
		"ó": "o", "ô": "o", "õ": "o",
		"ú": "u", "û": "u",
		"ç": "c",
	}
	
	for old, new := range replacements {
		titulo = strings.ReplaceAll(titulo, old, new)
	}
	
	return titulo
}

// ObterRelevancia retorna a relevância de um título
func (s *RelevanciaService) ObterRelevancia(titulo string) int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	tituloNorm := s.normalizarTitulo(titulo)
	
	if item, exists := s.data.ItensRelevancia[tituloNorm]; exists {
		return item.Relevancia
	}
	
	return 0 // Relevância padrão para itens não encontrados
}

// ObterEstatisticas retorna estatísticas do serviço de relevância
func (s *RelevanciaService) ObterEstatisticas() map[string]interface{} {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	stats := map[string]interface{}{
		"total_itens":        len(s.data.ItensRelevancia),
		"ultima_atualizacao": s.data.UltimaAtualizacao,
		"fontes": map[string]int{
			"1746":           0,
			"carioca-digital": 0,
			"ambos":          0,
		},
	}
	
	for _, item := range s.data.ItensRelevancia {
		if fontes, ok := stats["fontes"].(map[string]int); ok {
			fontes[item.Fonte]++
		}
	}
	
	return stats
}

// iniciarAtualizacaoAutomatica inicia a rotina de atualização automática
func (s *RelevanciaService) iniciarAtualizacaoAutomatica() {
	if s.config.IntervaloAtualizacao <= 0 {
		return
	}
	
	ticker := time.NewTicker(time.Duration(s.config.IntervaloAtualizacao) * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		log.Println("Iniciando atualização automática de dados de relevância...")
		if err := s.CarregarDados(); err != nil {
			log.Printf("Erro na atualização automática: %v", err)
		}
	}
}

// ExportarDados exporta os dados de relevância para JSON (para debug)
func (s *RelevanciaService) ExportarDados() ([]byte, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	return json.MarshalIndent(s.data, "", "  ")
} 

// resolvePath resolve path relativo para absoluto se necessário
func (s *RelevanciaService) resolvePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("caminho vazio")
	}
	
	// Se já é absoluto, usa como está
	if filepath.IsAbs(path) {
		return path, nil
	}
	
	// Tenta path relativo ao diretório atual
	if _, err := os.Stat(path); err == nil {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("erro ao converter para path absoluto: %v", err)
		}
		return absPath, nil
	}
	
	// Tenta path relativo ao diretório do executável
	ex, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("erro ao obter executável: %v", err)
	}
	
	exPath := filepath.Dir(ex)
	fullPath := filepath.Join(exPath, path)
	
	if _, err := os.Stat(fullPath); err == nil {
		return fullPath, nil
	}
	
	// Tenta path relativo ao diretório pai do executável (para casos onde o executável está em /bin ou /cmd)
	parentPath := filepath.Join(filepath.Dir(exPath), path)
	if _, err := os.Stat(parentPath); err == nil {
		return parentPath, nil
	}
	
	return "", fmt.Errorf("arquivo não encontrado: %s (tentado: %s, %s, %s)", path, path, fullPath, parentPath)
} 