package plugins

import (
	"log"
	"github.com/reddec/container"
	"os"
	"github.com/pkg/errors"
	"path/filepath"
	"net/http"
	"bytes"
	"github.com/Masterminds/sprig"
	"html/template"
	"io"
	"time"
	"context"
)

type Http struct {
	URL         string            `yaml:"url" mapstructure:"url"`         // template URL string
	Method      string            `yaml:"method"`                         // default POST
	Headers     map[string]string `yaml:"headers" mapstructure:"headers"` // additional header (non-template)
	Services    []string          `yaml:"services"`
	Timeout     time.Duration     `yaml:"timeout"`
	withTemplate                  `mapstructure:",squash" yaml:",inline"`
	log         *log.Logger       `yaml:"-"`
	servicesSet map[string]bool
	workDir     string
}

func (c *Http) renderAndSend(message string, params map[string]interface{}) {
	c.log.Println(message)

	tpl, err := template.New("").Funcs(sprig.FuncMap()).Parse(string(c.URL))
	if err != nil {
		c.log.Println("failed parse URL as template:", err)
		return
	}
	urlM := &bytes.Buffer{}
	err = tpl.Execute(urlM, params)
	if err != nil {
		c.log.Println("failed execute URL as template:", err)
		return
	}

	req, err := http.NewRequest(c.Method, urlM.String(), bytes.NewBufferString(message))
	if err != nil {
		c.log.Println("failed prepare request:", err)
		return
	}

	ctx, closer := context.WithTimeout(context.Background(), c.Timeout)
	defer closer()

	res, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		c.log.Println("failed make request:", err)
		return
	}
	io.Copy(os.Stdout, res.Body) // allow keep-alive
	res.Body.Close()
}

func (c *Http) Spawned(runnable container.Runnable, id container.ID) {
	if c.servicesSet[runnable.Label()] {
		content, params, renderErr := c.renderDefaultParams("spawned", string(id), runnable.Label(), nil, c.log)
		if renderErr != nil {
			c.log.Println("failed render:", renderErr)
		} else {
			c.renderAndSend(content, params)
		}
	}
}

func (c *Http) Prepare() error {
	c.servicesSet = makeSet(c.Services)
	c.log = log.New(os.Stderr, "[http] ", log.LstdFlags)
	if c.Method == "" {
		c.Method = "POST"
	}
	if c.Timeout == 0 {
		c.Timeout = 20 * time.Second
	}
	return nil
}

func (c *Http) Stopped(runnable container.Runnable, id container.ID, err error) {
	if c.servicesSet[runnable.Label()] {
		content, params, renderErr := c.renderDefaultParams("stopped", string(id), runnable.Label(), err, c.log)
		if renderErr != nil {
			c.log.Println("failed render:", renderErr)
		} else {
			c.renderAndSend(content, params)
		}
	}
}

func (a *Http) MergeFrom(other interface{}) (error) {
	b := other.(*Http)
	if a.URL == "" {
		a.URL = b.URL
	}
	if a.URL != b.URL {
		return errors.New("different urls")
	}
	if a.Method == "" {
		a.Method = b.Method
	}
	if a.Method != b.Method {
		return errors.New("different methods")
	}
	if a.Timeout == 0 {
		a.Timeout = b.Timeout
	}
	if a.Timeout != b.Timeout {
		return errors.New("different timeout")
	}
	a.withTemplate.resolvePath(a.workDir)
	b.withTemplate.resolvePath(b.workDir)
	if err := a.withTemplate.MergeFrom(&b.withTemplate); err != nil {
		return err
	}
	if a.Headers == nil {
		a.Headers = make(map[string]string)
	}
	for k, v := range b.Headers {
		a.Headers[k] = v
	}
	a.Services = append(a.Services, b.Services...)
	return nil
}

func init() {
	registerPlugin("http", func(file string) PluginConfig {
		return &Http{workDir: filepath.Dir(file)}
	})
}
