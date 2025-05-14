// internal/storage/file_storage.go
package storage

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sync"
    "time"
)

// FileStore implementa o armazenamento de sessões em arquivos
type FileStore struct {
    sessionDir string
    mutex      sync.RWMutex
    sessions   map[string]*SessionData
}

// SessionData mantém os dados de uma sessão
type SessionData struct {
    ID        string
    JID       string
    Data      []byte
    CreatedAt int64
    UpdatedAt int64
}

// NewFileStore cria um novo armazenamento baseado em arquivo
func NewFileStore(sessionDir string) (*FileStore, error) {
    // Garantir que o diretório existe
    if err := os.MkdirAll(sessionDir, 0755); err != nil {
        return nil, fmt.Errorf("falha ao criar diretório de sessões: %w", err)
    }
    
    store := &FileStore{
        sessionDir: sessionDir,
        sessions:   make(map[string]*SessionData),
    }
    
    // Carregar sessões existentes
    if err := store.loadSessions(); err != nil {
        return nil, err
    }
    
    return store, nil
}

// loadSessions carrega as sessões existentes do diretório
func (s *FileStore) loadSessions() error {
    files, err := os.ReadDir(s.sessionDir)
    if err != nil {
        return fmt.Errorf("falha ao ler diretório de sessões: %w", err)
    }
    
    for _, file := range files {
        if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
            continue
        }
        
        path := filepath.Join(s.sessionDir, file.Name())
        data, err := os.ReadFile(path)
        if err != nil {
            return fmt.Errorf("falha ao ler arquivo de sessão %s: %w", path, err)
        }
        
        var session SessionData
        if err := json.Unmarshal(data, &session); err != nil {
            return fmt.Errorf("falha ao decodificar sessão %s: %w", path, err)
        }
        
        s.sessions[session.ID] = &session
    }
    
    return nil
}

// SaveSession salva uma sessão no sistema de arquivos
func (s *FileStore) SaveSession(id string, data []byte) error {
    s.mutex.Lock()
    defer s.mutex.Unlock()
    
    session := &SessionData{
        ID:        id,
        Data:      data,
        UpdatedAt: time.Now().Unix(),
    }
    
    // Atualizar no cache
    s.sessions[id] = session
    
    // Salvar no arquivo
    path := filepath.Join(s.sessionDir, id+".json")
    jsonData, err := json.Marshal(session)
    if err != nil {
        return fmt.Errorf("falha ao codificar sessão: %w", err)
    }
    
    if err := os.WriteFile(path, jsonData, 0644); err != nil {
        return fmt.Errorf("falha ao salvar sessão: %w", err)
    }
    
    return nil
}

// GetSession obtém uma sessão do sistema de arquivos
func (s *FileStore) GetSession(id string) ([]byte, error) {
    s.mutex.RLock()
    defer s.mutex.RUnlock()
    
    session, exists := s.sessions[id]
    if !exists {
        return nil, fmt.Errorf("sessão não encontrada: %s", id)
    }
    
    return session.Data, nil
}

// DeleteSession remove uma sessão
func (s *FileStore) DeleteSession(id string) error {
    s.mutex.Lock()
    defer s.mutex.Unlock()
    
    // Remover do cache
    delete(s.sessions, id)
    
    // Remover do sistema de arquivos
    path := filepath.Join(s.sessionDir, id+".json")
    if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
        return fmt.Errorf("falha ao remover sessão: %w", err)
    }
    
    return nil
}