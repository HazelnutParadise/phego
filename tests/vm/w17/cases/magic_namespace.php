<?php
namespace Deep\Space\Nine;
echo __NAMESPACE__, "\n";
function station()
{
    return __FUNCTION__ . "|" . __METHOD__ . "|" . __NAMESPACE__;
}
echo station(), "\n";
echo __LINE__, " ", __DIR__, " ", __FILE__, "\n";
echo "[", __CLASS__, __TRAIT__, __PROPERTY__, "]\n";
const WHERE = __NAMESPACE__ . "\\" . __LINE__;
echo WHERE, "\n";
