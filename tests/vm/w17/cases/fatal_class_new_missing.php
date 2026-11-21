<?php
function build($name)
{
    echo "building ", $name, "\n";
    return new $name(1, 2);
}
class Exists
{
}
build("Exists");
build("DoesNotExist");
