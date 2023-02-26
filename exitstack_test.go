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

package exitstack_test

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	assertPkg "github.com/stretchr/testify/assert"

	"github.com/aezhar/exitstack"
)

type testResource struct {
	tracker *testTracker

	closeErr    error
	closeCalled bool

	openCalled bool
	openErr    error
}

func (f *testResource) Close() error {
	f.closeCalled = true
	f.tracker.trace = append(f.tracker.trace, f)
	return f.closeErr
}

func (f *testResource) openFn() (io.Closer, error) {
	f.openCalled = true
	return f, f.openErr
}

type testTracker struct {
	trace []*testResource
}

func (t *testTracker) r(openErr error, closeErr error) *testResource {
	return &testResource{
		tracker:  t,
		closeErr: closeErr,
		openErr:  openErr,
	}
}

func TestAddOpenFn(t *testing.T) {
	t.Run("golden", func(t *testing.T) {
		assert := assertPkg.New(t)

		var (
			st exitstack.S
			tt testTracker
		)

		rA := tt.r(nil, nil)
		rB := tt.r(nil, nil)
		rC := tt.r(nil, nil)
		rD := tt.r(nil, nil)

		assert.NoError(st.AddOpenFn(rA.openFn))
		assert.NoError(st.AddOpenFn(rB.openFn, rC.openFn))
		assert.NoError(st.AddOpenFn(rD.openFn))

		assert.NoError(st.Close())

		// Assert that all resources were closed in the
		// correct order.
		assert.Equal([]*testResource{rD, rC, rB, rA}, tt.trace)
	})

	t.Run("error", func(t *testing.T) {
		assert := assertPkg.New(t)

		var (
			st exitstack.S
			tr testTracker
		)

		rA := tr.r(nil, nil)
		rB := tr.r(fs.ErrNotExist, nil)
		rC := tr.r(nil, nil)

		assert.NoError(st.AddOpenFn(rA.openFn))
		err := st.AddOpenFn(rB.openFn, rC.openFn)

		assert.ErrorIs(err, fs.ErrNotExist)

		// assert A was opened but not closed yet.
		assert.True(rA.openCalled)
		assert.False(rA.closeCalled)

		// assert B was opened but not closed since open failed.
		assert.True(rB.openCalled)
		assert.False(rB.closeCalled)

		// assert C was not opened and not closed since B open failed.
		assert.False(rC.openCalled)
		assert.False(rC.closeCalled)

		assert.NoError(st.Close())

		// assert A was closed now.
		assert.True(rA.closeCalled)

		// assert B was not closed since opening B failed.
		assert.False(rB.closeCalled)

		// assert C was not opened and not closed since B open failed.
		assert.False(rC.openCalled)
		assert.False(rC.closeCalled)
	})
}

func TestClose(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		// Closing a nil exitstack.S should not cause any errors.
		var st exitstack.S

		assertPkg.Nil(t, st)
		assertPkg.NoError(t, st.Close())
	})

	t.Run("defer set to nil", func(t *testing.T) {
		// Closing a nil exitstack.S in defer that was set to nil
		// before should not close any open resource.
		var (
			st exitstack.S
			tr testTracker
		)

		r := tr.r(nil, nil)

		func() {
			defer st.Close()

			_, err := exitstack.Open(&st, r.openFn)
			assertPkg.NoError(t, err)

			st = nil
		}()

		assertPkg.False(t, r.closeCalled)
	})

	t.Run("error", func(t *testing.T) {
		assert := assertPkg.New(t)

		var (
			st exitstack.S
			tr testTracker
		)

		rA := tr.r(nil, nil)
		rB := tr.r(nil, syscall.EIO)
		rC := tr.r(nil, syscall.EBADF)
		rD := tr.r(nil, nil)

		err := st.AddOpenFn(rA.openFn, rB.openFn, rC.openFn, rD.openFn)
		assert.NoError(err)

		// Assert no error is lost.
		err = st.Close()

		assert.ErrorContains(err, "bad file descriptor")
		assert.ErrorContains(err, "input/output error")

		var errs interface{ Unwrap() []error }
		if assert.True(errors.As(err, &errs)) {
			assert.Equal([]error{syscall.EBADF, syscall.EIO}, errs.Unwrap())
		}

		// Assert that all close functions have been called
		// regardless of any previous close function returning
		// an error.
		assert.True(rA.closeCalled)
		assert.True(rB.closeCalled)
		assert.True(rC.closeCalled)
		assert.True(rD.closeCalled)
	})
}

func TestPushCloser(t *testing.T) {
	assert := assertPkg.New(t)

	var (
		st exitstack.S
		tt testTracker
	)

	rA := tt.r(nil, nil)
	rB := tt.r(nil, nil)
	rC := tt.r(nil, nil)
	rD := tt.r(nil, nil)

	st.AddCloser(rA)
	st.AddCloser(rB, rC)
	st.AddCloser(rD)

	assert.NoError(st.Close())

	assert.Equal([]*testResource{rD, rC, rB, rA}, tt.trace)
}

func TestPushCb(t *testing.T) {
	var called bool
	var st exitstack.S

	st.AddCb(func() error {
		called = true
		return nil
	})

	assertPkg.NoError(t, st.Close())

	assertPkg.True(t, called)
}

func TestOpen(t *testing.T) {
	assert := assertPkg.New(t)

	var st exitstack.S

	t.TempDir()

	f, err := exitstack.Open(&st, func() (*os.File, error) {
		return os.Create(filepath.Join(t.TempDir(), "tmpfile"))
	})
	if !assert.NoError(err) {
		return
	}

	_, err = f.Seek(0, io.SeekCurrent)
	if !assert.NoError(err) {
		return
	}

	if !assert.NoError(st.Close()) {
		return
	}

	_, err = f.Seek(0, io.SeekCurrent)
	var perr *fs.PathError
	if !(assert.True(errors.As(err, &perr)) && assert.ErrorContains(perr, "file already closed")) {
		return
	}
}
