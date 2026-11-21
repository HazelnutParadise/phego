<?php
class Account
{
    public $owner = "ann";
    private $pin = "1234";
}
$a = new Account;
echo "owner ", $a->owner, "\n";
echo $a->pin, "\n";
