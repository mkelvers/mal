package templates

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"path/filepath"
	"sync"
)

var (
	once     sync.Once
	renderer *Renderer
)

type Renderer struct {
	templates map[string]*template.Template
}

func GetRenderer() *Renderer {
	once.Do(func() {
		renderer = &Renderer{
			templates: make(map[string]*template.Template),
		}

		funcs := template.FuncMap{
			"dict": func(values ...any) map[string]any {
				m := make(map[string]any)
				for i := 0; i < len(values)-1; i += 2 {
					key, ok := values[i].(string)
					if !ok {
						continue
					}
					m[key] = values[i+1]
				}
				return m
			},
			"json": func(v any) template.HTMLAttr {
				b, _ := json.Marshal(v)
				return template.HTMLAttr(b)
			},
		}

		pages, err := filepath.Glob(filepath.Join(".", "templates", "*.gohtml"))
		if err != nil {
			log.Fatalf("failed to glob page templates: %v", err)
		}

		components, err := filepath.Glob(filepath.Join(".", "templates", "components", "*.gohtml"))
		if err != nil {
			log.Fatalf("failed to glob component templates: %v", err)
		}

		for _, page := range pages {
			name := filepath.Base(page)
			if name == "base.gohtml" {
				continue
			}

			tmpl := template.New(name).Funcs(funcs)
			// Parse base first so it establishes the core definitions
			tmpl = template.Must(tmpl.ParseFiles(filepath.Join(".", "templates", "base.gohtml")))

			// Parse all components next so they are available to the page
			if len(components) > 0 {
				tmpl = template.Must(tmpl.ParseFiles(components...))
			}

			// Parse the page itself last
			tmpl = template.Must(tmpl.ParseFiles(page))

			renderer.templates[name] = tmpl
			log.Printf("Loaded page template: %s", name)
		}
	})
	return renderer
}

func (r *Renderer) ExecuteTemplate(wr io.Writer, name string, data any) error {
	tmpl, ok := r.templates[name]
	if !ok {
		return fmt.Errorf("template %s not found", name)
	}
	return tmpl.ExecuteTemplate(wr, "base.gohtml", data)
}

func (r *Renderer) ExecuteFragment(wr io.Writer, name string, block string, data any) error {
	tmpl, ok := r.templates[name]
	if !ok {
		return fmt.Errorf("template %s not found", name)
	}
	return tmpl.ExecuteTemplate(wr, block, data)
}
