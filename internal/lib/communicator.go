package lib

/*
#cgo CFLAGS: -I${SRCDIR}/../../../libcommunicator/include
#cgo LDFLAGS: -L${SRCDIR}/../../../libcommunicator/target/release -lcommunicator
#include <stdlib.h>
#include "communicator.h"

// Callback bridge function for Go
extern void go_message_callback_bridge(char* author, char* content, void* user_data);
*/
import "C"
import (
	"fmt"
	"sync"
	"unsafe"
)

// Context represents a libcommunicator context
type Context struct {
	handle C.CommunicatorContext
	id     string
}

// MessageCallback function type for receiving messages
type MessageCallback func(author, content string)

var (
	callbackMutex sync.RWMutex
	callbacks     = make(map[string]MessageCallback)
)

// Initialize the library
func Initialize() error {
	if code := C.communicator_init(); code != C.COMMUNICATOR_SUCCESS {
		return fmt.Errorf("failed to initialize libcommunicator: %s", getErrorString(code))
	}
	return nil
}

// Cleanup the library
func Cleanup() {
	C.communicator_cleanup()
}

// CreateContext creates a new communicator context
func CreateContext(id string) (*Context, error) {
	cId := C.CString(id)
	defer C.free(unsafe.Pointer(cId))

	handle := C.communicator_context_create(cId)
	if handle == nil {
		return nil, fmt.Errorf("failed to create context")
	}

	return &Context{
		handle: handle,
		id:     id,
	}, nil
}

// Initialize initializes the context
func (c *Context) Initialize() error {
	if code := C.communicator_context_initialize(c.handle); code != C.COMMUNICATOR_SUCCESS {
		return fmt.Errorf("failed to initialize context: %s", getErrorString(code))
	}
	return nil
}

// SetConfig sets a configuration value
func (c *Context) SetConfig(key, value string) error {
	cKey := C.CString(key)
	cValue := C.CString(value)
	defer C.free(unsafe.Pointer(cKey))
	defer C.free(unsafe.Pointer(cValue))

	if code := C.communicator_context_set_config(c.handle, cKey, cValue); code != C.COMMUNICATOR_SUCCESS {
		return fmt.Errorf("failed to set config: %s", getErrorString(code))
	}
	return nil
}

// SetMessageCallback sets a callback for receiving messages
func (c *Context) SetMessageCallback(callback MessageCallback) {
	callbackMutex.Lock()
	callbacks[c.id] = callback
	callbackMutex.Unlock()
}

// SendMessage sends a message to a user (stub - would need actual libcommunicator messaging API)
func (c *Context) SendMessage(username, content string) error {
	// This would use actual libcommunicator messaging functions
	// For now, return success as the API is not fully implemented in libcommunicator yet
	return nil
}

// Destroy destroys the context and frees resources
func (c *Context) Destroy() {
	if c.handle != nil {
		C.communicator_context_destroy(c.handle)
		c.handle = nil
		
		callbackMutex.Lock()
		delete(callbacks, c.id)
		callbackMutex.Unlock()
	}
}

// Greet returns a greeting message from libcommunicator
func Greet(name string) string {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	cGreeting := C.communicator_greet(cName)
	if cGreeting == nil {
		return ""
	}
	defer C.communicator_free_string(cGreeting)

	return C.GoString(cGreeting)
}

// GetVersion returns the libcommunicator version
func GetVersion() string {
	return C.GoString(C.communicator_version())
}

func getErrorString(code C.CommunicatorErrorCode) string {
	return C.GoString(C.communicator_error_code_string(code))
}

//export go_message_callback_bridge
func go_message_callback_bridge(author *C.char, content *C.char, userData unsafe.Pointer) {
	if author == nil || content == nil {
		return
	}
	
	contextId := C.GoString((*C.char)(userData))
	
	callbackMutex.RLock()
	callback, exists := callbacks[contextId]
	callbackMutex.RUnlock()
	
	if exists {
		callback(C.GoString(author), C.GoString(content))
	}
}
