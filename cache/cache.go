package cache

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
)

type CachedResponse struct {
	StatusCode int
	Body       []byte
	Headers    map[string]string
}

type SpecResponse struct {
	StatusCode  int               `json:"status_code"`
	ContentFile string            `json:"content"`
	Headers     map[string]string `json:"headers"`
}

type Spec struct {
	SpecResponse `json:"response"`
	Key          string `json:"key"`
}

type Keyer interface {
	Key(r *http.Request) string
}

type Cacher interface {
	Keyer
	Get(key string) *CachedResponse
	Put(key string, r *httptest.ResponseRecorder)
}

type DefaultKeyer struct {
}

func (k DefaultKeyer) Key(r *http.Request) string {
	key := r.URL.RequestURI() + r.Method
	if strings.ToLower(r.Header.Get("chameleon-hash-body")) == "true" {
		defer r.Body.Close()
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		key += string(body)
	}

	hasher := md5.New()
	hasher.Write([]byte(key))
	return hex.EncodeToString(hasher.Sum(nil))
}

type diskCacher struct {
	DefaultKeyer
	cache    map[string]*CachedResponse
	dataDir  string
	specPath string
}

func NewDiskCacher(dataDir string) diskCacher {

	dc := diskCacher{
		cache:    nil,
		dataDir:  dataDir,
		specPath: path.Join(dataDir, "spec.json"),
	}

	cache := make(map[string]*CachedResponse)
	specs := dc.loadSpecs()

	for _, spec := range specs {
		body, err := ioutil.ReadFile(path.Join(dataDir, spec.SpecResponse.ContentFile))
		if err != nil {
			panic(err)
		}
		response := &CachedResponse{
			StatusCode: spec.StatusCode,
			Headers:    spec.Headers,
			Body:       body,
		}
		cache[spec.Key] = response
	}

	dc.cache = cache
	return dc
}

func (c diskCacher) Get(key string) *CachedResponse {
	return c.cache[key]
}

func (c diskCacher) loadSpecs() []Spec {
	specContent, err := ioutil.ReadFile(c.specPath)
	if err != nil {
		specContent = []byte{'[', ']'}
	}

	var specs []Spec
	err = json.Unmarshal(specContent, &specs)
	if err != nil {
		panic(err)
	}

	return specs
}

func (c diskCacher) Put(key string, resp *httptest.ResponseRecorder) {
	specs := c.loadSpecs()

	specHeaders := make(map[string]string)
	for k, v := range resp.Header() {
		specHeaders[k] = strings.Join(v, ", ")
	}

	newSpec := Spec{
		Key: key,
		SpecResponse: SpecResponse{
			StatusCode:  resp.Code,
			ContentFile: key,
			Headers:     specHeaders,
		},
	}

	specs = append(specs, newSpec)

	contentFilePath := path.Join(c.dataDir, key)
	err := ioutil.WriteFile(contentFilePath, resp.Body.Bytes(), 0644)
	if err != nil {
		panic(err)
	}

	specBytes, err := json.MarshalIndent(specs, "", "    ")
	err = ioutil.WriteFile(c.specPath, specBytes, 0644)
	if err != nil {
		panic(err)
	}

	c.cache[key] = &CachedResponse{
		StatusCode: resp.Code,
		Headers:    specHeaders,
		Body:       resp.Body.Bytes(),
	}
}