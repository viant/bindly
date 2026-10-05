package request

import bodyprovider "github.com/viant/bindly/provider/body"

type Decoder = bodyprovider.Decoder
type DecoderFunc = bodyprovider.DecoderFunc
type FieldDecoder = bodyprovider.FieldDecoder
type DecoderRegistry = bodyprovider.DecoderRegistry

func NewDecoderRegistry() *DecoderRegistry { return bodyprovider.NewDecoderRegistry() }
