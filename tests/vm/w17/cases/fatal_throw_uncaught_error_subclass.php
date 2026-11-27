<?php
// fatal_throw_uncaught_error_subclass: uncaught user Error subclass with a code, thrown from a method called from a function
class AppError extends Error {}

class Svc
{
    public function run($label, $count)
    {
        throw new AppError("svc failed", 12);
    }
}

function go($label, $count)
{
    $svc = new Svc();
    return $svc->run($label, $count);
}

echo "start\n";
go("text", 3);
echo "never\n";
