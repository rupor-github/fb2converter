package toml

import (
	"bytes"

	"fb2converter/go-micro/config/encoder"
	"github.com/BurntSushi/toml"
)

type tomlEncoder struct{}

func (t tomlEncoder) Encode(v interface{}) ([]byte, error) {
	b := bytes.NewBuffer(nil)
	defer b.Reset()
	err := toml.NewEncoder(b).Encode(v)
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (t tomlEncoder) Decode(d []byte, v interface{}) error {
	return toml.Unmarshal(d, v)
}

func (t tomlEncoder) String() string {
	return "toml"
}

func NewEncoder() encoder.Encoder {
	return tomlEncoder{}
}
