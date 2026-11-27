<?php
// fatal_throw_uncaught_tostring_throws: uncaught user exception whose own __toString throws
class Broken extends Exception
{
    public function __toString(): string
    {
        throw new LogicException("tostring failed");
    }
}

$b = new Broken("original");
echo "message: ", $b->getMessage(), "\n";
echo "code: ", $b->getCode(), "\n";
echo "class: ", $b::class, "\n";
echo "start\n";
throw $b;
echo "never\n";
