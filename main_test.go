package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type RoundTripFunc func(req *http.Request) *http.Response

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func NewTestClient(fn RoundTripFunc) *http.Client {
	return &http.Client{
		Transport: RoundTripFunc(fn),
	}
}

func getTestHttpClient(t *testing.T, expectedBody string) *http.Client {
	return NewTestClient(func(req *http.Request) *http.Response {
		desiredUrl := "http://localhost/random/next?min=0&max=255&num=3"
		if req.URL.String() != desiredUrl {
			t.Errorf("RandomSvc requeasted wrong URL. Got %s wanted %s", req.URL.String(), desiredUrl)
		}
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(expectedBody)),
			Header:     make(http.Header),
		}
	})
}

func getTestRandomService(t *testing.T, expectedNumbers []int) RandomService {
	initConfig()
	expectedResponse := fmt.Sprintf(`{"values":%s}`, strings.Join(strings.Fields(fmt.Sprint(expectedNumbers)), ","))
	return RandomService{BaseUrl: config.RandomServiceBaseUrl, Client: getTestHttpClient(t, expectedResponse)}
}

func TestRandomColorHandler(t *testing.T) {
	// Replace randomSvc in main with custom transport layer to mock RandomService responses

	desiredNumbers := []int{255, 0, 50}
	randomSvc = getTestRandomService(t, desiredNumbers)

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler := http.HandlerFunc(randomColorHandler)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	desiredNumbersHex := fmt.Sprintf("%02x%02x%02x", desiredNumbers[0], desiredNumbers[1], desiredNumbers[2])
	expected := fmt.Sprintf(`{"hex":"#%s","r":%d,"g":%d,"b":%d}`, desiredNumbersHex, desiredNumbers[0], desiredNumbers[1], desiredNumbers[2])
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}
