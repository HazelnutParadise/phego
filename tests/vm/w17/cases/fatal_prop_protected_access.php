<?php
class Account
{
    protected $balance = 100;
}
$a = new Account;
echo "isset is silent: ", (isset($a->balance) ? "yes" : "no"), "\n";
echo $a->balance, "\n";
