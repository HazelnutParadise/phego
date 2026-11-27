<?php
// finally_nested: try/finally nested in try, in catch and in finally, three levels, with exceptions at each level; prints the order
class L1 extends Exception {}
class L2 extends Exception {}
class L3 extends Exception {}

function level1()
{
    try {
        echo "L1 try\n";
        try {
            echo "L1 inner try\n";
            throw new L1("from inner");
        } finally {
            echo "L1 inner finally\n";
        }
    } catch (L1 $e) {
        echo "L1 catch: ", $e->getMessage(), "\n";
        try {
            throw new L2("from catch");
        } catch (L2 $e2) {
            echo "L1 nested catch: ", $e2->getMessage(), "\n";
        } finally {
            echo "L1 catch-level finally\n";
        }
    } finally {
        echo "L1 outer finally\n";
    }
    echo "L1 end\n";
}
level1();

function level2()
{
    try {
        try {
            throw new L2("two");
        } finally {
            echo "L2 inner finally\n";
            try {
                throw new L3("in finally");
            } catch (L3 $e) {
                echo "L2 finally caught: ", $e->getMessage(), "\n";
            }
        }
    } catch (L2 $e) {
        echo "L2 outer catch: ", $e->getMessage(), "\n";
    }
}
level2();

function level3()
{
    try {
        try {
            try {
                throw new L3("deep");
            } finally {
                echo "L3 depth3 finally\n";
            }
        } finally {
            echo "L3 depth2 finally\n";
        }
    } catch (L3 $e) {
        echo "L3 top catch: ", $e->getMessage(), "\n";
    } finally {
        echo "L3 top finally\n";
    }
}
level3();
echo "end\n";
