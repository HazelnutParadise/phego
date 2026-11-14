<?php
echo __LINE__, "\n";
echo __FILE__, "\n";
echo __DIR__, "\n";
echo "[", __NAMESPACE__, "][", __FUNCTION__, "][", __CLASS__, "][", __METHOD__, "][", __TRAIT__, "][", __PROPERTY__, "]\n";
$line = __LINE__;
echo $line + 1, " ", __LINE__ * 2, "\n";
echo __FILE__ . ":" . __LINE__, "\n";
$where = [__FILE__, __LINE__, __DIR__];
echo $where[1], " ", $where[2] === __DIR__ ? "same" : "diff", "\n";
echo __LINE__ === 11 ? "int" : "other", "\n";
