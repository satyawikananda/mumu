// Package library is the domain service for the user's favorite channels,
// recently watched channels and the resume reference.
//
// It maps stored rows to catalog entries so Discover, Watch and the Library
// share one identity (Entry.Key) and one notion of a playable channel. It owns
// no UI, performs no network I/O and never writes catalog data: favorites and
// history are the only user intent it changes.
package library
