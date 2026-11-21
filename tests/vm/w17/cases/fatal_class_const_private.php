<?php
class Vault
{
    private const CODE = "1234";
    public static function hint()
    {
        return "starts with " . self::CODE[0];
    }
}
echo Vault::hint(), "\n";
echo Vault::CODE, "\n";
