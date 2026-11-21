<?php
echo "start\n";
interface Maker
{
    public static function make();
}
class Impl implements Maker
{
    public function make()
    {
    }
}
