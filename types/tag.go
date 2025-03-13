package types

import "reflect"

type TagGenerator func(field *reflect.StructField, embedder Embedder) reflect.StructTag
