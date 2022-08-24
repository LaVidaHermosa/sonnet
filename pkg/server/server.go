// jsonnet-language-server: A Language Server Protocol server for Jsonnet.
// Copyright (C) 2021 Jack Baldry

// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.

// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package server

import (
	"context"
	"path/filepath"

	"github.com/google/go-jsonnet"
	"github.com/google/go-jsonnet/formatter"
	"github.com/grafana/jsonnet-language-server/pkg/stdlib"
	"github.com/grafana/jsonnet-language-server/pkg/utils"
	tankaJsonnet "github.com/grafana/tanka/pkg/jsonnet"
	"github.com/grafana/tanka/pkg/jsonnet/jpath"
	"github.com/jdbaldry/go-language-server-protocol/lsp/protocol"
	log "github.com/sirupsen/logrus"
)

const (
	errorRetrievingDocument = "unable to retrieve document from the cache"
)

// New returns a new language server.
func NewServer(name, version string, client protocol.ClientCloser) *server {
	server := &server{
		name:    name,
		version: version,
		cache:   newCache(),
		client:  client,
		fmtOpts: formatter.DefaultOptions(),
	}

	return server
}

// server is the Jsonnet language server.
type server struct {
	name, version string

	stdlib  []stdlib.Function
	cache   *cache
	client  protocol.ClientCloser
	getVM   func(path string) (*jsonnet.VM, error)
	extVars map[string]string
	fmtOpts formatter.Options

	// Feature flags
	EvalDiags bool
	LintDiags bool
}

func (s *server) WithStaticVM(jpaths []string) *server {
	log.Infof("Using the following jpaths: %v", jpaths)
	s.getVM = func(path string) (*jsonnet.VM, error) {
		jpaths = append(jpaths, filepath.Dir(path))
		vm := jsonnet.MakeVM()
		resetExtVars(vm, s.extVars)
		importer := &jsonnet.FileImporter{JPaths: jpaths}
		vm.Importer(importer)
		return vm, nil
	}
	return s
}

func (s *server) WithTankaVM(fallbackJPath []string) *server {
	log.Infof("Using tanka mode. Will fall back to the following jpaths: %v", fallbackJPath)
	s.getVM = func(path string) (*jsonnet.VM, error) {
		jpath, _, _, err := jpath.Resolve(path, false)
		if err != nil {
			log.Debugf("Unable to resolve jpath for %s: %s", path, err)
			jpath = append(fallbackJPath, filepath.Dir(path))
		}
		opts := tankaJsonnet.Opts{
			ImportPaths: jpath,
		}
		vm := tankaJsonnet.MakeVM(opts)
		resetExtVars(vm, s.extVars)
		return vm, nil
	}
	return s
}

func (s *server) DidChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {
	defer s.queueDiagnostics(params.TextDocument.URI)

	doc, err := s.cache.get(params.TextDocument.URI)
	if err != nil {
		return utils.LogErrorf("DidChange: %s: %w", errorRetrievingDocument, err)
	}

	if params.TextDocument.Version > doc.item.Version && len(params.ContentChanges) != 0 {
		doc.item.Text = params.ContentChanges[len(params.ContentChanges)-1].Text
		doc.ast, doc.err = jsonnet.SnippetToAST(doc.item.URI.SpanURI().Filename(), doc.item.Text)
		if doc.err != nil {
			return s.cache.put(doc)
		}
	}
	return nil
}

func (s *server) DidOpen(ctx context.Context, params *protocol.DidOpenTextDocumentParams) (err error) {
	defer s.queueDiagnostics(params.TextDocument.URI)

	doc := &document{item: params.TextDocument}
	if params.TextDocument.Text != "" {
		doc.ast, doc.err = jsonnet.SnippetToAST(params.TextDocument.URI.SpanURI().Filename(), params.TextDocument.Text)
		if doc.err != nil {
			return s.cache.put(doc)
		}
	}
	return s.cache.put(doc)
}

func (s *server) Initialize(ctx context.Context, params *protocol.ParamInitialize) (*protocol.InitializeResult, error) {
	log.Infof("Initializing %s version %s", s.name, s.version)

	s.diagnosticsLoop()

	var err error

	if s.stdlib == nil {
		log.Infoln("Reading stdlib")
		if s.stdlib, err = stdlib.Functions(); err != nil {
			return nil, err
		}
	}

	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			CompletionProvider:         protocol.CompletionOptions{TriggerCharacters: []string{"."}},
			HoverProvider:              true,
			DefinitionProvider:         true,
			DocumentFormattingProvider: true,
			ExecuteCommandProvider:     protocol.ExecuteCommandOptions{Commands: []string{}},
			TextDocumentSync: &protocol.TextDocumentSyncOptions{
				Change:    protocol.Full,
				OpenClose: true,
				Save: protocol.SaveOptions{
					IncludeText: false,
				},
			},
		},
		ServerInfo: struct {
			Name    string `json:"name"`
			Version string `json:"version,omitempty"`
		}{
			Name:    s.name,
			Version: s.version,
		},
	}, nil
}
