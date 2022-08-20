# exitstack

The package `exitstack` provides an exit stack that allows the combination of cleanup functions beyond the boundaries of a single function, and therefore extends the functionality and the reach of a single defer statement.

This library is inspired by the [ExitStack](https://docs.python.org/3/library/contextlib.html#contextlib.ExitStack) found in the Python standard library.
