<?php
// try_catch_namespaced: inside namespace App a bare Exception means App\Exception; \Exception and a use import match the global class
namespace App;

use LogicException;

class ModuleError extends \RuntimeException {}

function fail($kind)
{
    if ($kind === 'global') {
        throw new \Exception("global exception");
    }
    if ($kind === 'module') {
        throw new ModuleError("module error");
    }
    if ($kind === 'logic') {
        throw new LogicException("imported logic");
    }
    return "ok";
}

try {
    try {
        fail('global');
    } catch (Exception $e) {
        echo "wrong: bare Exception caught\n";
    }
} catch (\Exception $e) {
    echo "outer \\Exception caught: ", $e->getMessage(), "\n";
}

try {
    fail('global');
} catch (\Exception $e) {
    echo "\\Exception caught: ", $e::class, "\n";
}

try {
    fail('logic');
} catch (LogicException $e) {
    echo "use import caught: ", $e::class, "\n";
}

try {
    fail('module');
} catch (ModuleError $e) {
    echo "namespaced class caught: ", $e::class, "\n";
}

try {
    fail('module');
} catch (\RuntimeException $e) {
    echo "global RuntimeException caught: ", $e->getMessage(), "\n";
}

$r = fail('none');
echo "no throw: ", $r, "\n";
