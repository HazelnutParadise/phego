<?php
// fatal_exception_private_redeclare: a subclass redeclares the inherited Exception::$message with a type (compile fatal)
// The check runs when the class is declared, before any code runs, so nothing is printed.
// A class that does not redeclare $message is not affected by this rule.
// The message names the property, the class and the parent class.
// The whole file fails to compile, so none of the code below runs and nothing is printed.
// PHP reports "Type of Mine::$message must be omitted to match the parent definition in class Exception".
// Exception::$message is an untyped protected property, so a redeclaration must not add a type.
// The report points at the class declaration line, not the property line.
class Mine extends Exception
{
    protected int $message = 0;
}

echo "not reached\n";
