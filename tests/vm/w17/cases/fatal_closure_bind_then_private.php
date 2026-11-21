<?php
// A static closure that uses no $this can be bound to null scope. Binding an
// object to a static closure gives null with a warning. Calling that null
// result is a fatal error.

class A
{
    private $secret = "s";

    public function get()
    {
        return "instance get";
    }
}

$static = static function () {
    return "static body";
};

echo "static closure runs: ", $static(), "\n";

$obj = new A();
$bound = $static->bindTo($obj);

echo "bind to object gives null: ", $bound === null ? "yes" : "no", "\n";

$ok = function () {
    return "plain body";
};
echo "plain closure runs: ", $ok(), "\n";

echo "before calling the null result\n";

echo "value: ", $bound(), "\n";

echo "not reached\n";
