<?php
class NotCallable
{
}
$o = new NotCallable;
echo "calling\n";
$o();
