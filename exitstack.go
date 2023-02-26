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
	"io"

	"go.uber.org/multierr"
)

// S is an exit stack that allows the combination of cleanup functions,
// useful for those that are optional, conditional and/or driven by
// input data.
//
// Cleanup functions will be called in the reversed order they were
// added to the exit stack, meaning that the first cleanup function
// added to the exit stack will be the last cleanup function to be
// called, when the stack is unwounded.
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
	if c != nil && c != (io.Closer)(nil) {
		*s = append(*s, c)
	}
}

// An OpenFn function opens a resource and returns a handler to
// the resource or an error.
type OpenFn[T any] func() (T, error)

// AddOpenFn adds the result of all [exitstack.OpenFn] functions to the exit stack.
//
// AddOpenFn iterates through all [exitstack.OpenFn] functions in fns,
// calls each function in the order they were passed and adds the returned
// [io.Closer] instances to the exit stack in case of their success.
//
// If any [exitstack.OpenFn] function returns an error, no further
// [exitstack.OpenFn] function will be called and the last error will
// be returned. Make sure to call [exitstack.S.Close] at the end of your
// function through the *defer* pattern or any other means.
func (s *S) AddOpenFn(fns ...OpenFn[io.Closer]) error {
	for _, fn := range fns {
		closer, err := fn()
		if err != nil {
			return err
		}

		s.add(closer)
	}
	return nil
}

// AddCloser adds any given [io.Closer] instance to the exit stack.
func (s *S) AddCloser(closers ...io.Closer) {
	for _, closer := range closers {
		s.add(closer)
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

// Open calls fn to open a given resource and returns the resource
// with its original type after adding it to the exit stack in case
// fn returns no error.
//
// Open passes the given [exitstack.OpenFn] function in fn
// through the [exitstack.S.AddOpenFn] function of the [exitstack.S]
// instance in s and returns the original resource.
func Open[T io.Closer](s *S, fn OpenFn[T]) (out T, err error) {
	err = s.AddOpenFn(func() (io.Closer, error) {
		// Assign out and err to the outer function results.
		out, err = fn()
		return out, err
	})
	return
}
