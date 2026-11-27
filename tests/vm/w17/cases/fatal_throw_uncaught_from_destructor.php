<?php
// fatal_throw_uncaught_from_destructor: a destructor throws during unset at top level and nothing catches it
class Boom
{
    public function __destruct()
    {
        echo "destructor started\n";
        throw new Exception("from destructor");
    }
}

$b = new Boom();
echo "object created\n";
echo "before unset\n";
unset($b);
echo "after unset is not reached\n";
