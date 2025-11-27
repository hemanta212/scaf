package golang

import (
	"errors"
	"testing"

	"github.com/rlch/scaf/language"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoLanguageName(t *testing.T) {
	t.Parallel()

	lang := New()
	assert.Equal(t, "go", lang.Name())
}

func TestLanguageRegistry(t *testing.T) {
	t.Parallel()

	// Go language should be auto-registered via init()
	lang := language.Get("go")
	require.NotNil(t, lang)
	assert.Equal(t, "go", lang.Name())

	// Non-existent language
	assert.Nil(t, language.Get("nonexistent"))

	// RegisteredLanguages should include "go"
	names := language.RegisteredLanguages()
	assert.Contains(t, names, "go")
}

func TestGoLanguageGenerate(t *testing.T) {
	t.Parallel()

	lang := New()

	// Without binding, should return ErrNoBinding
	ctx := &language.GenerateContext{
		Suite: nil,
	}

	_, err := lang.Generate(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoBinding))
}

func TestGoLanguageGenerateWithContext(t *testing.T) {
	t.Parallel()

	lang := New()

	// Without binding, should return ErrNoBinding
	ctx := &Context{
		GenerateContext: language.GenerateContext{
			Suite: nil,
		},
		PackageName: "testpkg",
		Binding:     nil,
	}

	_, err := lang.GenerateWithContext(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoBinding))
}
