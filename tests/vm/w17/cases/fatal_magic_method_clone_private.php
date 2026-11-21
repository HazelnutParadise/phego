<?php
class Singleton
{
    private static $instance;
    public static function get()
    {
        return self::$instance ??= new self;
    }
    private function __clone()
    {
    }
}
echo (Singleton::get() === Singleton::get() ? "same instance" : "different"), "\n";
$copy = clone Singleton::get();
