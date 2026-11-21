<?php
class Money
{
}
function total()
{
    $sum = 10;
    $sum *= new Money;
    return $sum;
}
echo "computing\n";
echo total(), "\n";
