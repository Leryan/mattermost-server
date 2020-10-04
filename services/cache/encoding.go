package cache

import (
	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/tinylib/msgp/msgp"
	"github.com/vmihailenco/msgpack/v5"
)

func Encode(value interface{}) ([]byte, error) {
	// We use a fast path for hot structs.
	if msgpVal, ok := value.(msgp.Marshaler); ok {
		return msgpVal.MarshalMsg(nil)
	} else {
		// Slow path for other structs.
		return msgpack.Marshal(value)
	}
}

func Decode(e *entry, value interface{}) error {
	// We use a fast path for hot structs.
	if msgpVal, ok := value.(msgp.Unmarshaler); ok {
		_, err := msgpVal.UnmarshalMsg(e.value)
		return err
	}

	// This is ugly and makes the cache package aware of the model package.
	// But this is due to 2 things.
	// 1. The msgp package works on methods on structs rather than functions.
	// 2. Our cache interface passes pointers to empty pointers, and not pointers
	// to values. This is mainly how all our model structs are passed around.
	// It might be technically possible to use values _just_ for hot structs
	// like these and then return a pointer while returning from the cache function,
	// but it will make the codebase inconsistent, and has some edge-cases to take care of.
	switch v := value.(type) {
	case **model.User:
		var u model.User
		_, err := u.UnmarshalMsg(e.value)
		*v = &u
		return err
	case **model.Session:
		var s model.Session
		_, err := s.UnmarshalMsg(e.value)
		*v = &s
		return err
	}

	// Slow path for other structs.
	return msgpack.Unmarshal(e.value, value)
}
