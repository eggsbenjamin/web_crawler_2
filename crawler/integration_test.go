// +build integration

package crawler

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIntegration(t *testing.T) {
	for _, uri := range []string{"/", "/one", "/two", "/three", "/four", "/five"} {
		http.HandleFunc(uri, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Add("Content-Type", "text/html")
			w.Write([]byte(`
			<html>
				<body>
					<h1>Test</h1>
					<a href="http://www.test.com">Link 1</a>
					<a href="/one">Link 1</a>
					<a href="/two">Link 1</a>
					<a href="/three">Link 1</a>
					<a href="/four">Link 1</a>
					<a href="/five">Link 1</a>
				</body>
			</html>
		`))
		})
	}

	go func() {
		log.Fatal(http.ListenAndServe(":7777", nil))
	}()

	for i := 0; i < 5; i++ {
		if _, err := http.Get("http://localhost:7777"); err != nil {
			log.Println("waiting for server...")
			time.Sleep(1 * time.Second)
		}
		log.Println("server listening on localhost:7777")
		break
	}

	expectedOutput, err := os.Open("./testdata/expected_output")
	require.NoError(t, err)
	dec := json.NewDecoder(expectedOutput)

	var expectedPages []*Page
	for dec.More() {
		var page *Page
		require.NoError(t, dec.Decode(&page))
		expectedPages = append(expectedPages, page)
	}

	expected := map[string][]string{}
	for _, page := range expectedPages {
		expected[page.URL.String()] = page.Links
	}

	c := New(1, http.DefaultClient)
	buf := bytes.Buffer{}
	dec = json.NewDecoder(&buf)

	var result []*Page
	require.NoError(t, c.Crawl("http://localhost:7777", &buf))
	for dec.More() {
		var page *Page
		require.NoError(t, dec.Decode(&page))
		result = append(result, page)
	}

	actual := map[string][]string{}
	for _, page := range result {
		actual[page.URL.String()] = page.Links
	}

	require.Equal(t, len(expected), len(actual))

	for url := range expected {
		require.ElementsMatch(t, expected[url], actual[url])
	}
}
