// Package runtimetest holds the tests of the runtime module (zeroparse.dev/zp/runtime) that need
// packages of this module. The runtime module requires nothing, so that a module that imports it
// with zp gen -runtime import gets no other module; these tests therefore live here.
package runtimetest
