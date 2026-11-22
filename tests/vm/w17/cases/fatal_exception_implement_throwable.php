<?php
// fatal_exception_implement_throwable: a class cannot implement Throwable directly (compile fatal)
// Throwable is an interface, but only the built-in Exception and Error classes implement it directly.
// PHP applies this check when it links the class declaration, before any code runs.
// Writing "class X extends Exception {}" avoids the error, because Exception already implements Throwable.
// The error line is the class declaration line.
// The file is compiled as a whole, so the echo below is never reached.
// The whole file fails to compile, so none of the code below runs and nothing is printed.
// PHP reports "Class X cannot implement interface Throwable, extend Exception or Error instead".
// Throwable is an interface that only the built-in Exception and Error hierarchies may use.
// A user exception class must extend Exception or Error, which provides the implementation.
// The class declaration below is the invalid part; the echo after it is never reached.
class X implements Throwable {}

echo "not reached\n";
