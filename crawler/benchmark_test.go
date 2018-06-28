package crawler

import (
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func BenchmarkCrawler(b *testing.B) {
	tests := []struct {
		title, url string
		workers    int
	}{
		{
			"1",
			"http://monzo.com",
			1,
		},
		{
			"10",
			"http://monzo.com",
			10,
		},
		{
			"100",
			"http://monzo.com",
			100,
		},
		{
			"1000",
			"http://monzo.com",
			1000,
		},
		{
			"2000",
			"http://monzo.com",
			2000,
		},
	}

	for _, tt := range tests {
		b.Run(tt.title, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				c := New(tt.workers, &http.Client{Timeout: time.Second * 2})
				require.NoError(b, c.Crawl(tt.url, ioutil.Discard))
			}
		})
	}
}
