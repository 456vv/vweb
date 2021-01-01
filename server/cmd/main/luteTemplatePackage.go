package main

import (
	"text/template"
    "github.com/456vv/vweb/v2/builtin"
    "github.com/88250/lute"
    lute_ast "github.com/88250/lute/ast"
    lute_parse "github.com/88250/lute/parse"
    lute_html "github.com/88250/lute/html"
    lute_render  "github.com/88250/lute/render"
)

func luteTemplatePackage() map[string]template.FuncMap{
return map[string]template.FuncMap{
	"lute":{
		"FileID":lute.FileID,
		"FilePath":lute.FilePath,
		"FormatNode":lute.FormatNode,
		"NewOptions":lute.NewOptions,
		"NormalizeLinkBase":lute.NormalizeLinkBase,
		"RenderNodeVditorIRBlockDOM":lute.RenderNodeVditorIRBlockDOM,
		"Lute":func(a ...interface{}) (retn *lute.Lute) {builtin.GoTypeTo(&retn)(a...);return retn},
		"New":lute.New,
		"Option":func(o func(lute *lute.Lute))lute.Option{return lute.Option(o)},
	},
	"lute/ast":{
		"WalkStop":lute_ast.WalkStop,
		"WalkSkipChildren":lute_ast.WalkSkipChildren,
		"WalkContinue":lute_ast.WalkContinue,
		"NewNodeID":lute_ast.NewNodeID,
		"Walk":lute_ast.Walk,
		"ListData":func(a ...interface{}) (retn *lute_ast.ListData) {builtin.GoTypeTo(&retn)(a...);return retn},
		"Node":func(a ...interface{}) (retn *lute_ast.Node) {builtin.GoTypeTo(&retn)(a...);return retn},
		"NodeType":func(NodeType int) lute_ast.NodeType {return lute_ast.NodeType(NodeType)},
		"Str2NodeType":lute_ast.Str2NodeType,
		"WalkStatus":func(WalkStatus int) lute_ast.WalkStatus {return lute_ast.WalkStatus(WalkStatus)},
		"Walker":func(a ...interface{}) (retn lute_ast.Walker) {builtin.GoTypeTo(&retn)(a...);return retn},
	},
	"lute/parse":{
		"AddAutoLinkDomainSuffix":lute_parse.AddAutoLinkDomainSuffix,
		"BlockquoteContinue":lute_parse.BlockquoteContinue,
		"CodeBlockContinue":lute_parse.CodeBlockContinue,
		"FootnotesContinue":lute_parse.FootnotesContinue,
		"HtmlBlockContinue":lute_parse.HtmlBlockContinue,
		"ListItemContinue":lute_parse.ListItemContinue,
		"MathBlockContinue":lute_parse.MathBlockContinue,
		"NewEmojis":lute_parse.NewEmojis,
		"ParagraphContinue":lute_parse.ParagraphContinue,
		"SuperBlockContinue":lute_parse.SuperBlockContinue,
		"YamlFrontMatterContinue":lute_parse.YamlFrontMatterContinue,
		"Context":func(a ...interface{}) (retn *lute_parse.Context) {builtin.GoTypeTo(&retn)(a...);return retn},
		"InlineContext":func(a ...interface{}) (retn *lute_parse.InlineContext) {builtin.GoTypeTo(&retn)(a...);return retn},
		"Options":func(a ...interface{}) (retn *lute_parse.Options) {builtin.GoTypeTo(&retn)(a...);return retn},
		"Tree":func(a ...interface{}) (retn *lute_parse.Tree) {builtin.GoTypeTo(&retn)(a...);return retn},
		"Inline":lute_parse.Inline,
		"Parse":lute_parse.Parse,
	},
	"lute/html":{
		"BadEntity":lute_html.BadEntity,
		"Entities":lute_html.Entities,
		"ErrBufferExceeded":lute_html.ErrBufferExceeded,
		"EncodeDestination":lute_html.EncodeDestination,
		"EscapeHTML":lute_html.EscapeHTML,
		"EscapeString":lute_html.EscapeString,
		"HtmlUnescapeString":lute_html.HtmlUnescapeString,
		"Render":lute_html.Render,
		"UnescapeBytes":lute_html.UnescapeBytes,
		"UnescapeHTML":lute_html.UnescapeHTML,
		"UnescapeString":lute_html.UnescapeString,
		"Attribute":func(a ...interface{}) (retn *lute_html.Attribute) {builtin.GoTypeTo(&retn)(a...);return retn},
		"Node":func(a ...interface{}) (retn *lute_html.Node) {builtin.GoTypeTo(&retn)(a...);return retn},
		"Parse":lute_html.Parse,
		"ParseFragment":lute_html.ParseFragment,
		"ParseFragmentWithOptions":lute_html.ParseFragmentWithOptions,
		"ParseWithOptions":lute_html.ParseWithOptions,
		"NodeType":func(NodeType uint32) lute_html.NodeType {return lute_html.NodeType(NodeType)},
		"ParseOption":func(a ...interface{}) (retn *lute_html.ParseOption) {builtin.GoTypeTo(&retn)(a...);return retn},
		"ParseOptionEnableScripting":lute_html.ParseOptionEnableScripting,
		"Token":func(a ...interface{}) (retn *lute_html.Token) {builtin.GoTypeTo(&retn)(a...);return retn},
		"TokenType":func(TokenType uint32) lute_html.TokenType {return lute_html.TokenType(TokenType)},
		"Tokenizer":func(a ...interface{}) (retn *lute_html.Tokenizer) {builtin.GoTypeTo(&retn)(a...);return retn},
		"NewTokenizer":lute_html.NewTokenizer,
		"NewTokenizerFragment":lute_html.NewTokenizerFragment,
	},
	"lute/render":{
		"ExtRendererFunc":func(a ...interface{}) (retn lute_render.ExtRendererFunc) {builtin.GoTypeTo(&retn)(a...);return retn},
	},
}
}