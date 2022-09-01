package position

import (
	"github.com/google/go-jsonnet/ast"
	"github.com/jdbaldry/go-language-server-protocol/lsp/protocol"
)

func NewProtocolRange(startLine, startCharacter, endLine, endCharacter int) protocol.Range {
	return protocol.Range{
		Start: protocol.Position{
			Character: uint32(startCharacter),
			Line:      uint32(startLine),
		},
		End: protocol.Position{
			Character: uint32(endCharacter),
			Line:      uint32(endLine),
		},
	}
}

// RangeASTToProtocol translates a ast.LocationRange to a protocol.Range.
// The former is one indexed and the latter is zero indexed.
func RangeASTToProtocol(lr ast.LocationRange) protocol.Range {
	return protocol.Range{
		Start: protocol.Position{
			Line:      uint32(lr.Begin.Line - 1),
			Character: uint32(lr.Begin.Column - 1),
		},
		End: protocol.Position{
			Line:      uint32(lr.End.Line - 1),
			Character: uint32(lr.End.Column - 1),
		},
	}
}
