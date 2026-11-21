<?php
trait T { function m() {} }
class A { use T { zz as yy; } }
echo "x\n";
