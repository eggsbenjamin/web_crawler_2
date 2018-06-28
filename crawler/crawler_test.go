package crawler

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"

	gomock "github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestGetPages(t *testing.T) {
	dummyURL, err := url.Parse("http://www.google.com")
	require.NoError(t, err)

	t.Run("http client error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockHTTPClient := NewMockhttpClient(ctrl)
		mockHTTPClient.EXPECT().Get(dummyURL.String()).Return(nil, errors.New("error"))

		URLChan := make(chan *url.URL)
		pageChan, errChan := getPages(mockHTTPClient, URLChan)

		URLChan <- dummyURL
		close(URLChan)

		err, ok := <-errChan
		require.Error(t, err)
		require.True(t, ok)

		_, ok = <-errChan
		require.False(t, ok)

		_, ok = <-pageChan
		require.False(t, ok)

		ctrl.Finish()
	})

	t.Run("http client error response code", func(t *testing.T) {
		errCodes := []int{400, 500}

		for _, code := range errCodes {
			ctrl := gomock.NewController(t)
			mockHTTPClient := NewMockhttpClient(ctrl)
			mockHTTPClient.EXPECT().Get(dummyURL.String()).Return(
				&http.Response{
					StatusCode: code,
					Body:       ioutil.NopCloser(&bytes.Buffer{}),
				},
				nil,
			)

			URLChan := make(chan *url.URL)
			pageChan, errChan := getPages(mockHTTPClient, URLChan)

			URLChan <- dummyURL
			close(URLChan)

			err, ok := <-errChan
			require.True(t, ok)
			require.Equal(t, ErrHttpStatusCode, errors.Cause(err))

			_, ok = <-errChan
			require.False(t, ok)

			_, ok = <-pageChan
			require.False(t, ok)

			ctrl.Finish()
		}
	})

	t.Run("success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockHTTPClient := NewMockhttpClient(ctrl)
		mockHTTPClient.EXPECT().Get(dummyURL.String()).Return(
			&http.Response{
				StatusCode: 200,
				Body: ioutil.NopCloser(
					bytes.NewBufferString(
						`
							<html>
								<body>
									<h1>Test</h1>
									<a href="http://www.test.com"></a>
									<a href="test"></a>
								</body>
							</html>
						`,
					),
				),
			},
			nil,
		)

		URLChan := make(chan *url.URL)
		pageChan, errChan := getPages(mockHTTPClient, URLChan)

		URLChan <- dummyURL
		close(URLChan)

		result, ok := <-pageChan
		require.True(t, ok)
		require.Equal(t, dummyURL, result.URL)

		links := []string{}
		for _, link := range result.Links {
			links = append(links, link.String())
		}
		require.Equal(t, []string{"http://www.test.com", "http://www.google.com/test"}, links)

		_, ok = <-errChan
		require.False(t, ok)

		ctrl.Finish()
	})
}

func TestCollectLinks(t *testing.T) {
	dummyURL, err := url.Parse("http://www.google.com")
	require.NoError(t, err)

	tests := []struct {
		title, html string
		expected    []string
	}{
		{
			"empty",
			"",
			[]string{},
		},
		{
			"no links",
			`<html><body><h1>test</h1></body></html>`,
			[]string{},
		},
		{
			"single",
			`<html><body><a href="test"></a></body></html>`,
			[]string{"http://www.google.com/test"},
		},
		{
			"multiple",
			`<html><body><a href="test1"></a><a href="test2"></a></body></html>`,
			[]string{"http://www.google.com/test1", "http://www.google.com/test2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			result := collectLinks(dummyURL, bytes.NewBufferString(tt.html))
			require.Equal(t, len(tt.expected), len(result))

			urls := []string{}
			for _, r := range result {
				urls = append(urls, r.String())
			}
			require.ElementsMatch(t, tt.expected, urls)
		})
	}
}

func TestFormatURL(t *testing.T) {
	dummyURL, err := url.Parse("http://www.google.com/one/two")
	require.NoError(t, err)

	t.Run("valid", func(t *testing.T) {
		tests := []struct {
			title, rawURL, expected string
		}{
			{
				"absolute",
				"http://www.test.com",
				"http://www.test.com",
			},
			{
				"relative",
				"test",
				"http://www.google.com/one/test",
			},
			{
				"relative parent",
				"../../test",
				"http://www.google.com/test",
			},
			{
				"root",
				"/test",
				"http://www.google.com/test",
			},
			{
				"anchor",
				"#test",
				"http://www.google.com/one/two",
			},
		}

		for _, tt := range tests {
			t.Run(tt.title, func(t *testing.T) {
				result := formatURL(dummyURL, tt.rawURL)
				require.Equal(t, tt.expected, result.String())
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		tests := []struct {
			title, rawURL string
		}{
			{
				"mailto",
				"mailto:test@test.com",
			},
		}

		for _, tt := range tests {
			t.Run(tt.title, func(t *testing.T) {
				require.Nil(t, formatURL(dummyURL, tt.rawURL))
			})
		}
	})
}
