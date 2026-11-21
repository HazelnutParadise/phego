<?php
class A { function __destruct() { echo "dtor\n"; } }
new A(nope());
