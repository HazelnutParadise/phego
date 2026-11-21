<?php
class Lazy
{
    public readonly string $name;
}
$l = new Lazy;
echo "initializing from outside\n";
$l->name = "x";
