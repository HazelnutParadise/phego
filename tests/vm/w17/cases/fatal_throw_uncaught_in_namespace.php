<?php
// fatal_throw_uncaught_in_namespace: uncaught namespaced user exception thrown from a namespaced function and method
namespace App\Core;

class Problem extends \Exception {}

class Worker
{
    public function run()
    {
        throw new Problem("namespaced method");
    }
}

function fail()
{
    $w = new Worker();
    $w->run();
}

echo "start\n";
fail();
echo "never\n";
