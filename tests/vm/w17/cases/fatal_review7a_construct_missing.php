<?php
class A { }
class B extends A { function __construct() { parent::__construct(); } }
new B;
