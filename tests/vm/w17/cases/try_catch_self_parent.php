<?php
// try_catch_self_parent: inside class methods, catch the class's own type and its parent type by their real names (catch (self) and catch (parent) are compile errors)
class BaseFault extends Exception {}

class ServiceFault extends BaseFault
{
    public static function run($kind)
    {
        try {
            self::raise($kind);
        } catch (ServiceFault $e) {
            return "own type caught: " . $e::class . " " . $e->getMessage();
        } catch (BaseFault $e) {
            return "parent type caught: " . $e::class . " " . $e->getMessage();
        }
        return "none";
    }

    public static function raise($kind)
    {
        if ($kind === 'service') {
            throw new ServiceFault("s");
        }
        if ($kind === 'deep') {
            throw new DeepFault("d");
        }
        if ($kind === 'base') {
            throw new BaseFault("b");
        }
        if ($kind === 'plain') {
            throw new Exception("p");
        }
        return "ok";
    }
}

class DeepFault extends ServiceFault {}

$r = ServiceFault::run('service');
echo "1: ", $r, "\n";
$r = ServiceFault::run('deep');
echo "2: ", $r, "\n";
$r = ServiceFault::run('base');
echo "3: ", $r, "\n";
$r = ServiceFault::run('ok');
echo "4: ", $r, "\n";

try {
    $r = ServiceFault::run('plain');
    echo "not reached: ", $r, "\n";
} catch (Exception $e) {
    echo "5 escaped: ", $e::class, " ", $e->getMessage(), "\n";
}
