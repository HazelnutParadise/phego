<?php
class Registry
{
    private static $secret = "s";
    public static function hint()
    {
        return self::$secret;
    }
}
echo Registry::hint(), "\n";
echo Registry::$secret, "\n";
