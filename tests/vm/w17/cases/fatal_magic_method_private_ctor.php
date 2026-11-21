<?php
class Factory
{
    private function __construct()
    {
        echo "constructed\n";
    }
    public static function make()
    {
        return new static;
    }
}
Factory::make();
$f = new Factory;
