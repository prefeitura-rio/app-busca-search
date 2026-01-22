package synonyms

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/typesense/typesense-go/v3/typesense"
	"github.com/typesense/typesense-go/v3/typesense/api"
)

// Service gerencia sinônimos no Typesense
type Service struct {
	client     *typesense.Client
	collection string
}

// NewService cria um novo serviço de sinônimos
func NewService(client *typesense.Client, collection string) *Service {
	return &Service{
		client:     client,
		collection: collection,
	}
}

// LoadDefaults carrega sinônimos padrão no Typesense
func (s *Service) LoadDefaults(ctx context.Context) error {
	log.Printf("Carregando sinônimos padrão para collection %s...", s.collection)

	loaded := 0
	for _, group := range DefaultSynonyms {
		err := s.UpsertSynonym(ctx, group.Root, group.Synonyms)
		if err != nil {
			log.Printf("Aviso: erro ao carregar sinônimo '%s': %v", group.Root, err)
			continue
		}
		loaded++
	}

	log.Printf("Sinônimos carregados: %d/%d", loaded, len(DefaultSynonyms))
	return nil
}

// UpsertSynonym cria ou atualiza um sinônimo
func (s *Service) UpsertSynonym(ctx context.Context, root string, synonyms []string) error {
	// ID único baseado no root
	id := sanitizeID(root)

	// Prepara lista de sinônimos (inclui root)
	allSynonyms := append([]string{root}, synonyms...)

	synonymSchema := &api.SearchSynonymSchema{
		Synonyms: allSynonyms,
	}

	_, err := s.client.Collection(s.collection).Synonyms().Upsert(ctx, id, synonymSchema)
	if err != nil {
		return fmt.Errorf("erro ao upsert sinônimo %s: %w", id, err)
	}

	return nil
}

// DeleteSynonym remove um sinônimo
func (s *Service) DeleteSynonym(ctx context.Context, root string) error {
	id := sanitizeID(root)
	_, err := s.client.Collection(s.collection).Synonym(id).Delete(ctx)
	if err != nil {
		return fmt.Errorf("erro ao deletar sinônimo %s: %w", id, err)
	}
	return nil
}

// ListSynonyms lista todos os sinônimos configurados
func (s *Service) ListSynonyms(ctx context.Context) ([]*api.SearchSynonym, error) {
	result, err := s.client.Collection(s.collection).Synonyms().Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar sinônimos: %w", err)
	}
	return result, nil
}

// ClearAll remove todos os sinônimos
func (s *Service) ClearAll(ctx context.Context) error {
	synonymList, err := s.ListSynonyms(ctx)
	if err != nil {
		return err
	}

	for _, syn := range synonymList {
		if syn != nil && syn.Id != nil {
			_, err := s.client.Collection(s.collection).Synonym(*syn.Id).Delete(ctx)
			if err != nil {
				log.Printf("Aviso: erro ao deletar sinônimo %s: %v", *syn.Id, err)
			}
		}
	}

	return nil
}

// sanitizeID converte texto para ID válido
func sanitizeID(s string) string {
	// Remove espaços e caracteres especiais
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")

	// Remove acentos básicos
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "ã", "a", "â", "a",
		"é", "e", "ê", "e",
		"í", "i",
		"ó", "o", "õ", "o", "ô", "o",
		"ú", "u", "ü", "u",
		"ç", "c",
	)
	s = replacer.Replace(s)

	return s
}
