<?php
function plain()
{
    return __FUNCTION__ . "|" . __METHOD__ . "|" . __CLASS__ . "|" . __LINE__;
}
echo plain(), "\n";
function Mixed_Case()
{
    return __FUNCTION__;
}
echo Mixed_Case(), "\n";
function with_default($name = __FUNCTION__, $line = __LINE__)
{
    return $name . ":" . $line;
}
echo with_default(), "\n";
echo with_default("given", 0), "\n";
function nested_call()
{
    return plain() . "/" . __FUNCTION__;
}
echo nested_call(), "\n";
echo "[", __FUNCTION__, "]\n";
