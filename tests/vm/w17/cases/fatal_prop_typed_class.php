<?php
class Engine
{
}
class Wheel
{
}
class Car
{
    public ?Engine $engine = null;
}
$c = new Car;
$c->engine = new Engine;
echo "engine set\n";
$c->engine = new Wheel;
