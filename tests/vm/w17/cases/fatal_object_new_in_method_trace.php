<?php
class Factory
{
    public static function build($what)
    {
        return new $what();
    }
}
echo "building\n";
$x = Factory::build("Ghost");
