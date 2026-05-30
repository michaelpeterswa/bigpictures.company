// Package problems centralizes RFC 9457 Problem Details construction across the
// CLI. Every package that returns errors should construct them via [New] with a
// type path under [Base], so the resulting URIs form a discoverable namespace.
//
// Returned *rfc9457.RFC9457 values satisfy the error interface, so they flow
// through normal Go error returns and can be matched with errors.As at the top
// level of the CLI for rendering.
package problems

import (
	"errors"

	"alpineworks.io/rfc9457"
)

// Base is the prefix every problem type URI shares.
const Base = "https://bigpictures.company/problems"

// New constructs a Problem with a fully-qualified type URI of Base/typePath.
// Additional rfc9457 options (status, instance, extensions) may be supplied;
// they override any defaults set here.
func New(typePath, title, detail string, opts ...rfc9457.RFC9457Option) *rfc9457.RFC9457 {
	base := []rfc9457.RFC9457Option{
		rfc9457.WithType(Base + "/" + typePath),
		rfc9457.WithTitle(title),
		rfc9457.WithDetail(detail),
	}
	return rfc9457.NewRFC9457(append(base, opts...)...)
}

// Ext is a thin alias for rfc9457.NewExtension so callers don't have to import
// rfc9457 directly for the common case.
func Ext(key string, value any) rfc9457.Extension {
	return rfc9457.NewExtension(key, value)
}

// WithExt is sugar for rfc9457.WithExtensions accepting variadic [Ext] values.
func WithExt(e ...rfc9457.Extension) rfc9457.RFC9457Option {
	return rfc9457.WithExtensions(e...)
}

// As walks the error chain looking for a *rfc9457.RFC9457. Sugar over
// errors.As that avoids forcing every caller to import rfc9457 just to declare
// a target variable.
func As(err error) (*rfc9457.RFC9457, bool) {
	var p *rfc9457.RFC9457
	if errors.As(err, &p) {
		return p, true
	}
	return nil, false
}
