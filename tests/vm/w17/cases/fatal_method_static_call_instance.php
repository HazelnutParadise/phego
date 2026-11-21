<?php
class Greeter
{
    public $name = "x";
    public function hello()
    {
        return "hello";
    }
}
echo (new Greeter)->hello(), "\n";
echo Greeter::hello(), "\n";
