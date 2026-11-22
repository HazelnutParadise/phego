<?php
// fatal_exception_override_final: a subclass cannot override the final method Exception::getMessage() (compile fatal)
// The error points at the method line, not at the class line.
// Exception::getMessage is final, so the subclass method is rejected when the class is declared.
// Calling $this->getMessage() from a subclass method is still allowed; only redefining it is not.
// Nothing is printed, because the file fails to compile.
// The whole file fails to compile, so none of the code below runs and nothing is printed.
// PHP reports "Cannot override final method Exception::getMessage()" on the method line.
// getMessage is declared final in the built-in Exception class, so no subclass may redefine it.
class Mine extends Exception
{
    public function getMessage(): string
    {
        return "overridden";
    }
}

echo "not reached\n";
