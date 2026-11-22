<?php
// finally_nested_returns: an inner finally returns while an outer finally is still pending; the outer finally can return something else
class Stop extends Exception {}

function variantA()
{
    try {
        try {
            return "A inner try";
        } finally {
            echo "A inner finally returns\n";
            return "A inner finally";
        }
    } finally {
        echo "A outer finally runs\n";
    }
}
$r = variantA();
echo "A: ", $r, "\n";

function variantB()
{
    try {
        try {
            return "B inner";
        } finally {
            echo "B inner finally\n";
        }
    } finally {
        echo "B outer finally returns\n";
        return "B outer finally";
    }
}
$r = variantB();
echo "B: ", $r, "\n";

function variantC()
{
    try {
        try {
            throw new Stop("C pending");
        } finally {
            echo "C inner finally returns\n";
            return "C inner finally";
        }
    } finally {
        echo "C outer finally\n";
    }
}
$r = variantC();
echo "C: ", $r, "\n";

function variantD()
{
    try {
        try {
            return "D";
        } finally {
            return "D inner";
        }
    } finally {
        echo "D outer finally throws\n";
        throw new Stop("D outer throw");
    }
}
try {
    $r = variantD();
    echo "not reached: ", $r, "\n";
} catch (Stop $e) {
    echo "D caller caught: ", $e->getMessage(), "\n";
}
