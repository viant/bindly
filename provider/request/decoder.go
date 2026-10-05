package request

import "github.com/viant/bindly/provider/body"

type Decoder = body.Decoder
type DecoderFunc = body.DecoderFunc
type FieldDecoder = body.FieldDecoder
type DecoderRegistry = body.DecoderRegistry

func NewDecoderRegistry() *DecoderRegistry { return body.NewDecoderRegistry() }
