<?php
function age(int $years)
{
    return $years;
}
function caller()
{
    return age("ten");
}
echo "age ", age(10), "\n";
caller();
