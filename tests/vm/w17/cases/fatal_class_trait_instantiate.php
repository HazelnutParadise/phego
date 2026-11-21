<?php
trait Helper
{
    public static function hi()
    {
        return "hi";
    }
}
echo "before\n";
$h = new Helper;
