package services

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// FilterService gerencia o filtro de IDs baseado no arquivo CSV
type FilterService struct {
	csvPath         string
	excludedIDs     map[string]bool
	lastModified    time.Time
	mu              sync.RWMutex
}

// NewFilterService cria uma nova instância do serviço de filtro
func NewFilterService(csvPath string) *FilterService {
	service := &FilterService{
		csvPath:     csvPath,
		excludedIDs: make(map[string]bool),
	}
	
	// Carrega os IDs na inicialização
	if err := service.loadExcludedIDs(); err != nil {
		log.Printf("Erro ao carregar IDs excluídos do CSV: %v", err)
	}
	
	return service
}

// loadExcludedIDs carrega os IDs do arquivo CSV para exclusão
func (f *FilterService) loadExcludedIDs() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	// Verifica se o arquivo existe
	if _, err := os.Stat(f.csvPath); os.IsNotExist(err) {
		log.Printf("Arquivo CSV de filtro não encontrado: %s", f.csvPath)
		return nil // Não é um erro crítico, o filtro simplesmente não funcionará
	}
	
	// Verifica se precisa recarregar baseado na data de modificação
	fileInfo, err := os.Stat(f.csvPath)
	if err != nil {
		return fmt.Errorf("erro ao obter informações do arquivo CSV: %v", err)
	}
	
	if !fileInfo.ModTime().After(f.lastModified) && len(f.excludedIDs) > 0 {
		// Arquivo não foi modificado desde a última carga
		return nil
	}
	
	file, err := os.Open(f.csvPath)
	if err != nil {
		return fmt.Errorf("erro ao abrir arquivo CSV: %v", err)
	}
	defer file.Close()
	
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("erro ao ler arquivo CSV: %v", err)
	}
	
	// Limpa o mapa existente
	f.excludedIDs = make(map[string]bool)
	
	// Pula o cabeçalho se existir
	if len(records) > 0 {
		for i, record := range records {
			if i == 0 {
				// Verifica se é realmente um cabeçalho (primeira linha)
				if len(record) > 0 && record[0] == "carioca_id" {
					continue
				}
			}
			
			// A primeira coluna deve conter o carioca_id
			if len(record) > 0 && record[0] != "" {
				f.excludedIDs[record[0]] = true
			}
		}
	}
	
	f.lastModified = fileInfo.ModTime()
	
	log.Printf("Carregados %d IDs para exclusão do arquivo %s", len(f.excludedIDs), f.csvPath)
	return nil
}

// ShouldExclude verifica se um ID deve ser excluído dos resultados
func (f *FilterService) ShouldExclude(id string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	return f.excludedIDs[id]
}

// ReloadIfNeeded recarrega os IDs se o arquivo foi modificado
func (f *FilterService) ReloadIfNeeded() error {
	return f.loadExcludedIDs()
}

// GetExcludedCount retorna o número de IDs excluídos carregados
func (f *FilterService) GetExcludedCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	return len(f.excludedIDs)
}

// FilterHits filtra uma lista de hits removendo aqueles com IDs excluídos
// Este método é específico para documentos da collection "carioca-digital"
func (f *FilterService) FilterHits(hits []map[string]interface{}, collectionName string) []map[string]interface{} {
	// Só aplica filtro na collection "carioca-digital"
	if collectionName != "carioca-digital" {
		return hits
	}
	
	// Recarrega IDs se necessário
	if err := f.ReloadIfNeeded(); err != nil {
		log.Printf("Erro ao recarregar IDs excluídos: %v", err)
	}
	
	filteredHits := make([]map[string]interface{}, 0, len(hits))
	
	for _, hit := range hits {
		shouldKeep := true
		
		// Extrai o ID do documento do hit
		if document, ok := hit["document"].(map[string]interface{}); ok {
			if id, ok := document["id"].(string); ok {
				if f.ShouldExclude(id) {
					shouldKeep = false
				}
			}
		}
		
		if shouldKeep {
			filteredHits = append(filteredHits, hit)
		}
	}
	
	return filteredHits
}

// FilterMultiCollectionHits filtra hits de múltiplas collections, aplicando filtro apenas na carioca-digital
func (f *FilterService) FilterMultiCollectionHits(hits []map[string]interface{}) []map[string]interface{} {
	// Recarrega IDs se necessário
	if err := f.ReloadIfNeeded(); err != nil {
		log.Printf("Erro ao recarregar IDs excluídos: %v", err)
	}
	
	filteredHits := make([]map[string]interface{}, 0, len(hits))
	
	for _, hit := range hits {
		shouldKeep := true
		
		// Verifica se é da collection carioca-digital
		if document, ok := hit["document"].(map[string]interface{}); ok {
			// Verifica se existe um campo que identifica a collection
			// Se for carioca-digital, aplica o filtro
			if id, ok := document["id"].(string); ok {
				// Assume que se o hit tem um ID e estamos em uma busca multi-collection,
				// precisamos verificar se vem da carioca-digital
				// Uma forma de identificar seria pela estrutura do ID ou por um campo específico
				
				// Por enquanto, vamos usar uma heurística: se o documento tem um campo
				// específico da carioca-digital ou se podemos identificar pela estrutura
				
				// Aqui assumimos que documentos da carioca-digital podem ser identificados
				// Você pode ajustar esta lógica conforme necessário
				if f.isFromCariocaDigital(document) && f.ShouldExclude(id) {
					shouldKeep = false
				}
			}
		}
		
		if shouldKeep {
			filteredHits = append(filteredHits, hit)
		}
	}
	
	return filteredHits
}

// isFromCariocaDigital verifica se um documento é da collection carioca-digital
// Esta é uma função auxiliar que pode ser customizada conforme a estrutura dos dados
func (f *FilterService) isFromCariocaDigital(document map[string]interface{}) bool {
	// Implementação pode ser baseada em:
	// 1. Campo específico que identifica a collection
	// 2. Estrutura específica do documento
	// 3. Padrão no ID
	
	// Por enquanto, retorna true para todos os documentos, já que o método
	// FilterMultiCollectionHits será usado quando sabemos que pode conter
	// documentos da carioca-digital
	
	// Se você tiver um campo que identifica a collection, use assim:
	// if collection, ok := document["collection"].(string); ok {
	//     return collection == "carioca-digital"
	// }
	
	// Por padrão, assume que pode ser da carioca-digital
	return true
} 