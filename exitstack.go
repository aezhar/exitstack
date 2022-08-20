// * * * * * * * * * * * * * * * * * * * * * * * * * * * * * * * *
// Copyright(c) 2022 individual contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// <https://www.apache.org/licenses/LICENSE-2.0>
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.
// * * * * * * * * * * * * * * * * * * * * * * * * * * * * * * * *

// Package exitstack provides an exit stack that allows the combination
// of cleanup functions beyond the boundaries of a single function, and
// therefore extends the functionality and the reach of a single defer
// statement.
package exitstack

import (
	"context"
	"io"

	"go.uber.org/multierr"
)

// S is an exit stack that allows the combination of cleanup functions,
// useful for those that are optional, conditional and/or driven by
// input data.
type S []io.Closer

// Close closes all elements on the exit stack and clears it.
//
// The elements in the exit stack are iterated in the reversed order
// to when they were added, clears the exit stack by assigning nil
// to the slice and returns any encountered errors.
func (s *S) Close() (err error) {
	if s != nil {
		for i := len(*s) - 1; i >= 0; i-- {
			err = multierr.Append(err, (*s)[i].Close())
		}
		*s = nil
	}
	return
}

func (s *S) add(c io.Closer) {
	*s = append(*s, c)
}

// An OpenFn function opens a resource and returns a handler to
// the resource or an error.
type OpenFn[T any] func(ctx context.Context) (T, error)

// AddOpenFn adds the result of all [exitstack.OpenFn] functions to the exit stack.
//
// AddOpenFn iterates through all [exitstack.OpenFn] functions in fns,
// calls each function in the order they were passed and adds the returned
// [io.Closer] instances to the exit stack in reversed order in case of success.
//
// If any [exitstack.OpenFn] function returns an error, the entire
// exit stack will be unwound and the accumulated errors will be returned.
func (s *S) AddOpenFn(ctx context.Context, fns ...OpenFn[io.Closer]) error {
	for _, fn := range fns {
		closer, err := fn(ctx)
		if err != nil {
			return multierr.Append(err, s.Close())
		}

		if closer != nil && closer != (io.Closer)(nil) {
			s.add(closer)
		}

		if ctx.Err() != nil {
			return multierr.Append(ctx.Err(), s.Close())
		}
	}
	return nil
}

// AddCloser adds any given [io.Closer] instance to the exit stack.
func (s *S) AddCloser(closers ...io.Closer) {
	for _, closer := range closers {
		if closer != nil && closer != (io.Closer)(nil) {
			s.add(closer)
		}
	}
}

type closerFn func() error

func (f closerFn) Close() error {
	return f()
}

// AddCb adds a callback to the exit stack that will be called when
// unwinding the exit stack.
func (s *S) AddCb(fn func() error) {
	if fn != nil {
		s.add(closerFn(fn))
	}
}

// OpenWithContext opens a given resource within a given [context.Context].
//
// OpenWithContext passes the given [exitstack.OpenFn] function in fn
// through the [exitstack.S.AddOpenFn] method of the [exitstack.S]
// instance in s and returns the original resource.
func OpenWithContext[T io.Closer](s *S, ctx context.Context, fn OpenFn[T]) (out T, err error) {
	err = s.AddOpenFn(ctx, func(ctx context.Context) (io.Closer, error) {
		out, err = fn(ctx)
		return out, err
	})
	return
}

// Open calls fn and returns the resource after adding it to the exit stack.
//
// See [exitstack.OpenWithContext] for more details.
func Open[T io.Closer](s *S, fn func() (T, error)) (T, error) {
	return OpenWithContext(s, context.Background(), func(context.Context) (T, error) {
		return fn()
	})
}
