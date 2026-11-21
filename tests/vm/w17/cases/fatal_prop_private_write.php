<?php
class Account
{
    private $pin = "1234";
}
function hack(Account $a)
{
    $a->pin = "0000";
}
echo "hacking\n";
hack(new Account);
