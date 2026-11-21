<?php
// A closure made outside the class reads a private property of an object
// passed in. Outside the class scope, PHP raises a fatal error.

class Vault
{
    private $secret = "gold";
    public $label = "public label";
}

$v = new Vault();
echo "public label: ", $v->label, "\n";

$public_reader = function ($o) {
    return $o->label;
};
echo "public read through closure: ", $public_reader($v), "\n";

$private_reader = function ($o) {
    return $o->secret;
};

echo "about to read a private property from outside the class\n";

echo "value: ", $private_reader($v), "\n";

echo "not reached\n";
