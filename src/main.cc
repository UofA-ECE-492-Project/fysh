/**
 * Copyright (C) 2024 Yahya Al-Shamali, Kyle Prince, Charles Ancheta
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */
/**
 * \file main.cc
 */
#include "AST/AST.h"
#include "Compyler/Compyler.h"
#include "Lexer/Lexer.h"
#include "Parser/Parser.h"
#include <fstream>
#include <getopt.h>
#include <iostream>
#include <llvm-18/llvm/Support/JSON.h>
#include <llvm/Support/raw_ostream.h>
#include <sstream>

struct Options {
  enum class Output { AST, IR } output;
  bool noOpt = false;
  std::string outputFilename = "-";
};

void compyle(std::istream &stream, Options opts) {
  std::stringstream ss;
  ss << stream.rdbuf();
  std::string source{ss.str()};
  fysh::FyshLexer lexer{source.data()};
  fysh::FyshParser parser{lexer};
  fysh::ast::FyshBlock program{parser.parseProgram()};

  if (opts.output == Options::Output::AST) {
    std::cout << program;
    return;
  }

  if (program.size() == 1) {
    if (const fysh::ast::Error *err =
            std::get_if<fysh::ast::Error>(&program[0])) {
      std::cerr << "Error: " << err->getraw() << std::endl;
      return;
    }
  }

  fysh::Compyler cumpyler;
  fysh::Program p{cumpyler.compyle(program, opts.noOpt)};
  if (p.empty()) {
    std::cerr << "error compyling?" << std::endl;
  } else {
    p.print(opts.outputFilename);
  }
}

Options parseOptions(int argc, char *argv[]) {
  Options opts;
  char c;
  while ((c = getopt(argc, argv, "o:nah")) != -1) {
    switch (c) {
    case 'a': {
      opts.output = Options::Output::AST;
      break;
    }
    case 'n': {
      opts.noOpt = true;
      break;
    }
    case 'o': {
      opts.outputFilename = optarg;
      break;
    }
    case 'h': {
      std::cout << "USAGE: " << argv[0] << "[-o OUTPUT] [-an] [INPUT]"
                << std::endl;
      std::exit(0);
    }
    default:
      break;
    }
  }

  return opts;
}

int main(int argc, char *argv[]) {
  Options opts = parseOptions(argc, argv);
  if (argv[optind] == NULL) {
    compyle(std::cin, opts);
  } else {
    std::ifstream inputFile(argv[optind]);
    if (!inputFile.is_open()) {
      std::cerr << "Error opening file " << argv[optind] << " for reading\n";
      return 1;
    }
    compyle(inputFile, opts);
  }
  return 0;
}
