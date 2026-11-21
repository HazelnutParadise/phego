<?php
class Greeter
{
    public function hello()
    {
        return "hello";
    }
}
$g = new Greeter;
echo $g->hello(), "\n";
echo $g->goodbye(), "\n";
