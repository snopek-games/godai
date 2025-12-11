package godot

import (
	"bufio"
	"godai/mcp/godot/variant"
	"io"
	"os"
	"strings"
)

type configFileSection struct {
	name   string
	values variant.OrderedMap[string, any]
}

type ConfigFile struct {
	// This is a comment header, like found at the top of `project.godot`
	header   []string
	sections []configFileSection
}

func NewConfigFile() *ConfigFile {
	c := &ConfigFile{}
	c.Clear()
	return c
}

func LoadConfigFile(path string) (*ConfigFile, error) {
	c := NewConfigFile()

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	w := bufio.NewReader(f)
	if err := c.readFrom(w); err != nil {
		return nil, err
	}

	return c, nil
}

func ReadConfigFile(r io.Reader) (*ConfigFile, error) {
	c := NewConfigFile()
	err := c.readFrom(r)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func ParseConfigFile(data string) (*ConfigFile, error) {
	c := NewConfigFile()

	r := strings.NewReader(data)
	if err := c.readFrom(r); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *ConfigFile) Clear() {
	c.header = make([]string, 0)
	c.sections = make([]configFileSection, 0, 1)
}

func (c *ConfigFile) ListSections() []string {
	l := []string{}
	for _, s := range c.sections {
		l = append(l, s.name)
	}
	return l
}

func (c *ConfigFile) ListKeys(section string) []string {
	l := []string{}
	for _, s := range c.sections {
		if s.name == section {
			for _, kv := range s.values {
				l = append(l, kv.Key)
			}
			break
		}
	}
	return l
}

func (c *ConfigFile) Get(section string, key string) (any, bool) {
	for _, s := range c.sections {
		if s.name == section {
			return s.values.Get(key)
		}
	}
	return nil, false
}

func (c *ConfigFile) Set(section string, key string, value any) {
	for i := range c.sections {
		s := &c.sections[i]
		if s.name == section {
			s.values.Set(key, value)
			return
		}
	}
	s := configFileSection{
		name:   section,
		values: make(variant.OrderedMap[string, any], 0, 1),
	}
	s.values.Set(key, value)
	if section == "" {
		c.sections = append([]configFileSection{s}, c.sections...)
	} else {
		c.sections = append(c.sections, s)
	}
}

func (c *ConfigFile) GetInt64(section string, key string) (int64, bool) {
	raw, ok := c.Get(section, key)
	if !ok {
		return 0, false
	}
	v, ok := raw.(int64)
	if !ok {
		return 0, false
	}
	return v, true
}

func (c *ConfigFile) GetFloat64(section string, key string) (float64, bool) {
	raw, ok := c.Get(section, key)
	if !ok {
		return 0.0, false
	}
	v, ok := raw.(float64)
	if !ok {
		return 0.0, false
	}
	return v, true
}

func (c *ConfigFile) GetString(section string, key string) (string, bool) {
	raw, ok := c.Get(section, key)
	if !ok {
		return "", false
	}
	v, ok := raw.(string)
	if !ok {
		return "", false
	}
	return v, true
}

func (c *ConfigFile) GetBool(section string, key string) (bool, bool) {
	raw, ok := c.Get(section, key)
	if !ok {
		return false, false
	}
	v, ok := raw.(bool)
	if !ok {
		return false, false
	}
	return v, true
}

func (c *ConfigFile) Write(w io.Writer) error {
	bw := bufio.NewWriter(w)
	vw := variant.NewWriter(bw)
	defer vw.Flush()

	if len(c.header) > 0 {
		for _, line := range c.header {
			if err := vw.WriteComment(line); err != nil {
				return err
			}
		}
	}

	for _, s := range c.sections {
		if s.name != "" {
			if err := vw.WriteTag(s.name, nil); err != nil {
				return err
			}
		}

		if _, err := bw.WriteRune('\n'); err != nil {
			return err
		}

		for _, v := range s.values {
			if err := vw.WriteAssignment(v.Key, v.Value); err != nil {
				return err
			}
		}

		if _, err := bw.WriteRune('\n'); err != nil {
			return err
		}
	}

	return nil
}

func (c *ConfigFile) WriteFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	if err := c.Write(w); err != nil {
		return err
	}

	return nil
}

func (c *ConfigFile) String() (string, error) {
	w := &strings.Builder{}
	if err := c.Write(w); err != nil {
		return "", err
	}
	return w.String(), nil
}

func (c *ConfigFile) readFrom(r io.Reader) error {
	p := variant.NewParser(r)
	p.SetSimpleTag(true)

	inHeader := true
	section := ""

	for {
		s, err := p.ParseStatement()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch s.Type {
		case variant.StatementTypeComment:
			if inHeader {
				c.header = append(c.header, s.Name)
			}
		case variant.StatementTypeTag:
			section = s.Name
			inHeader = false
		case variant.StatementTypeAssignment:
			c.Set(section, s.Name, s.Value)
			inHeader = false
		}
	}

	return nil
}
