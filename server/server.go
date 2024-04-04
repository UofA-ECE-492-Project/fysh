package server

import (
	ctx "context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/Fysh-Fyve/fyshls/support"
	"github.com/Fysh-Fyve/fyshls/version"
	fysh "github.com/Fysh-Fyve/tree-sitter-fysh"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
	"github.com/tliron/glsp/server"
)

func getLogger(file string) io.WriteCloser {
	if version.LogStderr == "true" && file == "-" {
		return os.Stderr
	} else {
		// file, err := os.CreateTemp(".", "fyshls")
		if file == "-" {
			file = "log.txt"
		}
		f, err := os.Create(file)
		if err != nil {
			panic(err)
		}
		return f
	}
}

func Run() {
	file := flag.String("output", "-", "log output destination")
	v := flag.Bool("version", false, "Print FyshLS version")
	_ = flag.Bool("stdio", true, "Make VS C*de stop erroring out")
	flag.Parse()
	if *v {
		fmt.Println("fyshls version", version.BuildVersion())
		return
	}
	w := getLogger(*file)
	defer w.Close()
	logger := log.New(w, "[fyshls] ", log.LstdFlags|log.Lshortfile)
	fysh := NewFyshLs(logger)
	fysh.RunStdio()
}

type Server struct {
	name      string
	version   string
	log       *log.Logger
	documents map[string][]byte
	trees     map[string]*sitter.Tree

	handler protocol.Handler
}

func NewFyshLs(logger *log.Logger) *Server {
	s := &Server{
		name:      "fyshls",
		version:   version.BuildVersion(),
		log:       logger,
		documents: map[string][]byte{},
		trees:     map[string]*sitter.Tree{},
	}

	s.handler = protocol.Handler{
		LogTrace:   s.logTrace,
		Initialize: s.initialize,
		Shutdown:   s.shutdown,

		TextDocumentDidOpen:   s.openDocument,
		TextDocumentDidSave:   s.saveDocument,
		TextDocumentDidChange: s.changeDocument,

		TextDocumentHover:      s.hover,
		TextDocumentDefinition: s.definition,
		TextDocumentCompletion: s.completion,

		TextDocumentSemanticTokensFull: s.semanticTokensFull,
	}
	return s
}

func (s *Server) logTrace(context *glsp.Context, params *protocol.LogTraceParams) error {
	s.log.Println(params.Message)
	return nil
}

func GetTree(content []byte) (*sitter.Tree, error) {
	p := sitter.NewParser()
	p.SetLanguage(fysh.GetLanguage())
	tree, err := p.ParseCtx(ctx.Background(), nil, content)
	if err != nil {
		return nil, err
	}
	return tree, nil
}

func (s *Server) saveDocument(
	context *glsp.Context,
	params *protocol.DidSaveTextDocumentParams,
) error {
	if params.Text != nil {
		return s.updateDoc(params.TextDocument.URI, *params.Text)
	}
	return nil
}

func (s *Server) definition(
	context *glsp.Context,
	params *protocol.DefinitionParams,
) (any, error) {
	return nil, nil
}

func (s *Server) hover(
	context *glsp.Context,
	params *protocol.HoverParams,
) (*protocol.Hover, error) {
	return nil, nil
}

func (s *Server) updateDoc(uri, text string) error {
	file := []byte(text)
	s.documents[uri] = file
	var err error
	if s.trees[uri], err = GetTree(file); err != nil {
		return fmt.Errorf("failed to get root node: %v", err)
	}
	return nil
}

func (s *Server) changeDocument(
	context *glsp.Context,
	params *protocol.DidChangeTextDocumentParams,
) error {
	for _, changes := range params.ContentChanges {
		c, ok := changes.(protocol.TextDocumentContentChangeEventWhole)
		if ok {
			err := s.updateDoc(params.TextDocument.URI, c.Text)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("change event not supported")
		}
	}
	return nil
}

func (s *Server) openDocument(
	context *glsp.Context,
	params *protocol.DidOpenTextDocumentParams,
) error {
	return s.updateDoc(params.TextDocument.URI, params.TextDocument.Text)
}

func getFysh(v int64) string {
	format, zero, one := "><%s°>", "(", "{"
	if v < 0 {
		v, format, zero, one = -v, "<°%s><", ")", "}"
	}
	binary := strconv.FormatInt(v, 2)
	return fmt.Sprintf(
		format,
		strings.ReplaceAll(strings.ReplaceAll(binary, "0", zero), "1", one),
	)
}

func (s *Server) completion(
	context *glsp.Context,
	params *protocol.CompletionParams,
) (any, error) {
	tree := s.trees[params.TextDocument.URI]
	n, err := getNodeFromPosition(tree.RootNode(), params.Position)
	if err != nil {
		if err == io.EOF {
			return []protocol.CompletionItem{}, nil
		} else {
			s.log.Println("completion: error iterating", err)
			return nil, err
		}
	}

	rang := protocol.Range{
		Start: toPosition(n.StartPoint()),
		End:   toPosition(n.EndPoint()),
	}
	text := n.Content(s.documents[params.TextDocument.URI])
	// Prepare for at least 1 completion item
	completionList := make([]protocol.CompletionItem, 0, 1)
	switch text {
	case "@":
		fallthrough
	case "^":
		fallthrough
	case "*":
		token := fmt.Sprintf("><(((%s>", text)
		item := protocol.CompletionItem{
			Label:    token,
			TextEdit: protocol.TextEdit{Range: rang, NewText: token},
		}
		completionList = append(completionList, item)
	default:
		if item, err := tryNumberCompletion(text, rang); err == nil {
			completionList = append(completionList, item)
		}
	}

	return completionList, nil
}

func (s *Server) semanticTokensFull(
	context *glsp.Context,
	params *protocol.SemanticTokensParams,
) (*protocol.SemanticTokens, error) {
	data := Encode(
		s.documents[params.TextDocument.URI],
		s.trees[params.TextDocument.URI],
	)
	// var lineNum uint32 = 0
	// for i := 0; i < len(data)/5; i++ {
	// 	lineNum += data[i*5]
	// 	s.log.Println(lineNum, data[i*5:i*5+5])
	// }
	return &protocol.SemanticTokens{Data: data}, nil
}

func (s *Server) initialize(
	context *glsp.Context,
	params *protocol.InitializeParams,
) (any, error) {
	capabilities := s.handler.CreateServerCapabilities()

	// FULL sync only
	capabilities.TextDocumentSync = 1
	capabilities.HoverProvider = true
	capabilities.DefinitionProvider = true
	capabilities.CompletionProvider = &protocol.CompletionOptions{
		TriggerCharacters: []string{"0", "1", "2", "3", "4", "5", "6", "7", "8",
			"9", "0", "-", "@", "^", "*"},
	}

	if params.Capabilities.TextDocument.SemanticTokens != nil {
		tokenTypes, _ := support.GetTokenTypes()
		tokenModifiers, _ := support.GetTokenModifiers()
		capabilities.SemanticTokensProvider = &protocol.SemanticTokensOptions{
			Full: true,
			Legend: protocol.SemanticTokensLegend{
				TokenTypes:     tokenTypes,
				TokenModifiers: tokenModifiers,
			},
		}
	}

	n, err := json.MarshalIndent(params, "", " ")
	if err != nil {
		s.log.Fatal(err)
	}
	s.log.Println(string(n))

	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    s.name,
			Version: &s.version,
		},
	}, nil
}

func (s *Server) shutdown(context *glsp.Context) error {
	return nil
}

func (s *Server) RunStdio() {
	s.log.Printf("%s - Starting server...", s.version)
	server := server.NewServer(&s.handler, s.name, true)
	server.RunStdio()
}
