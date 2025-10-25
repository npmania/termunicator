package lib

/*
#cgo CFLAGS: -I${SRCDIR}/../../../libcommunicator/include
#cgo LDFLAGS: -L${SRCDIR}/../../../libcommunicator/target/release -lcommunicator
#include <stdlib.h>
#include "communicator.h"
*/
import "C"
import (
	"unsafe"
)

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
