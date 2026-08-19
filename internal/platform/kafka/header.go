package kafka

import (
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"
)

type HeaderCarrier struct {
	headers *[]kgo.RecordHeader
}

func NewHeaderCarrier(
	headers *[]kgo.RecordHeader,
) *HeaderCarrier {
	return &HeaderCarrier{
		headers: headers,
	}
}

func (c *HeaderCarrier) Get(key string) string {
	for _, header := range *c.headers {
		if strings.EqualFold(header.Key, key) {
			return string(header.Value)
		}
	}

	return ""
}

func (c *HeaderCarrier) Set(key, value string) {
	*c.headers = append(
		*c.headers,
		kgo.RecordHeader{
			Key:   key,
			Value: []byte(value),
		},
	)
}

func (c *HeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(*c.headers))

	for _, header := range *c.headers {
		keys = append(keys, header.Key)
	}

	return keys
}
