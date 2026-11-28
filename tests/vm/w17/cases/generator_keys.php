<?php
function gen() { yield 'a'; yield 5 => 'b'; yield 'c'; yield 'k' => 'd'; yield 'e'; yield -10 => 'f'; yield 'g'; yield "7" => 'h'; yield 'i'; yield null => 'j'; yield 'l'; yield 3 => 'm'; yield 'n'; }
foreach (gen() as $k => $v) { echo gettype_ish($k), " $v\n"; }
function gettype_ish($k) { if ($k === null) return "NULL"; if ($k === (int)$k) return "int($k)"; return "string($k)"; }
