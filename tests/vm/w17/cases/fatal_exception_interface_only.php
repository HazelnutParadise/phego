<?php
// fatal_exception_interface_only: a class implementing a user interface that extends Throwable, without extending Exception or Error (compile fatal)
// The interface MyThrowable is valid on its own, because an interface may extend Throwable.
// The failure appears only when Impl is declared, since Impl inherits Throwable through MyThrowable.
// A fix is to make Impl extend Exception or Error.
// The whole file fails to compile, so none of the code below runs and nothing is printed.
// PHP reports "Class Impl cannot implement interface Throwable, extend Exception or Error instead".
// The problem is inherited: MyThrowable extends Throwable, so Impl implements Throwable too.
// Extending a user interface does not avoid the rule; the class must extend Exception or Error.
// The interface declaration by itself is valid; only the class that implements it fails.
interface MyThrowable extends Throwable {}

class Impl implements MyThrowable {}

echo "not reached\n";
