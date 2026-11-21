<?php
abstract class Base
{
    abstract public static function make();
    public static function build()
    {
        return self::make();
    }
}
echo "building\n";
Base::build();
