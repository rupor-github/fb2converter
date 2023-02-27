cmake_minimum_required(VERSION 3.5)

project(fb2converter NONE)

message(STATUS "Running cmake version ${CMAKE_VERSION}")
if (NOT CMAKE_BUILD_TYPE)
    set(CMAKE_BUILD_TYPE "Release" CACHE STRING "default to release build type" FORCE)
endif()
set_property(CACHE CMAKE_BUILD_TYPE PROPERTY STRINGS "Debug" "Release")
message(STATUS "Build type: ${CMAKE_BUILD_TYPE}")
message(STATUS "Generator: ${CMAKE_GENERATOR}")

set(CMAKE_MODULE_PATH ${CMAKE_MODULE_PATH} ${CMAKE_SOURCE_DIR}/cmake)

if (NOT "$ENV{MSYSTEM_NAME}" STREQUAL "")
    set(MSYSTEM_NAME "$ENV{MSYSTEM_NAME}" CACHE INTERNAL "Copied from environment")
else()
    set(MSYSTEM_NAME "linux_amd64" CACHE INTERNAL "Host environment")
endif()

if(MSYSTEM_NAME STREQUAL "win32")
    set(KINDLEGEN "${PROJECT_SOURCE_DIR}/kindlegen/windows/kindlegen.exe")
    set(CROSS_GOOS windows)
    set(CROSS_GOARCH 386)
    set(INSTALL_SUBDIR "/fb2converter")
    set(CROSS_EXECUTABLE_SUFFIX ".exe") # we are not using cmake cross compilation abilities
    message(STATUS "Targeting 32 bits Windows executable")
elseif(MSYSTEM_NAME STREQUAL "win64")
    set(KINDLEGEN "${PROJECT_SOURCE_DIR}/kindlegen/windows/kindlegen.exe")
    set(CROSS_GOOS windows)
    set(CROSS_GOARCH amd64)
    set(INSTALL_SUBDIR "/fb2converter")
    set(CROSS_EXECUTABLE_SUFFIX ".exe") # we are not using cmake cross compilation abilities
    message(STATUS "Targeting 64 bits Windows executable")
elseif(MSYSTEM_NAME STREQUAL "darwin_amd64")
    set(KINDLEGEN "${PROJECT_SOURCE_DIR}/kindlegen/macos/kindlegen")
    set(CROSS_GOOS darwin)
    set(CROSS_GOARCH amd64)
    message(STATUS "Targeting 64 bits Darwin AMD executable")
elseif(MSYSTEM_NAME STREQUAL "darwin_arm64")
    set(KINDLEGEN "${PROJECT_SOURCE_DIR}/kindlegen/macos/kindlegen")  # will this work?
    set(CROSS_GOOS darwin)
    set(CROSS_GOARCH arm64)
    message(STATUS "Targeting 64 bits Darwin ARM executable")
elseif(MSYSTEM_NAME STREQUAL "linux_arm64")
    set(CROSS_GOOS linux)
    set(CROSS_GOARCH arm64)
    message(STATUS "Targeting 64 bits Linux ARM executable")
elseif(MSYSTEM_NAME STREQUAL "linux_i386")
    set(KINDLEGEN "${PROJECT_SOURCE_DIR}/kindlegen/linux/kindlegen")
    set(CROSS_GOOS linux)
    set(CROSS_GOARCH 386)
    message(STATUS "Targeting 32 bits Linux executable")
elseif(MSYSTEM_NAME STREQUAL "linux_amd64")
    # host platform is always linux amd64
    set(KINDLEGEN "${PROJECT_SOURCE_DIR}/kindlegen/linux/kindlegen")
    message(STATUS "Targeting 64 bits Linux executable")
else()
    message(FATAL_ERROR "Currently unsuppored environment \"${MSYSTEM_NAME}\".")
endif()

find_package(Go REQUIRED)
find_package(Git REQUIRED)

# Make sure we setup go properly
if (DEFINED ENV{GOPATH})
    set(GO_PATH "$ENV{GOPATH}")
else()
    set(GO_PATH "/tmp/gopkg_${CMAKE_PROJECT_NAME}")
    execute_process(COMMAND ${CMAKE_COMMAND} -E make_directory ${GO_PATH})
endif()
message(STATUS "Using go modules with GOPATH=${GO_PATH}")
if(GO_PATH MATCHES "^${PROJECT_SOURCE_DIR}.*")
    message(FATAL_ERROR "We are working in \"go modules outside of GOPATH mode\" - GOPATH should be outside of project source directory!")
endif()

########################################################################################################
# Setting up versioning
########################################################################################################

# Project version number - comes from release version tag
set(GIT_REF $ENV{GITHUB_REF})
if(GIT_REF MATCHES "refs/tags/v([0-9]+)\\.([0-9]+)\\.?([0-9]*[-a-zA-Z0-9+]*)")
    set(PRJ_VERSION_MAJOR ${CMAKE_MATCH_1})
    set(PRJ_VERSION_MINOR ${CMAKE_MATCH_2})
    set(PRJ_VERSION_PATCH ${CMAKE_MATCH_3})
else()
    set(PRJ_VERSION_MAJOR 0)
    set(PRJ_VERSION_MINOR 0)
    set(PRJ_VERSION_PATCH "0-dev")
endif()

if(DEFINED ENV{GITHUB_SHA})
    set(GIT_HASH "$ENV{GITHUB_SHA}")
elseif(EXISTS "${PROJECT_SOURCE_DIR}/.git" AND IS_DIRECTORY "${PROJECT_SOURCE_DIR}/.git")
    execute_process(COMMAND ${CMAKE_SOURCE_DIR}/cmake/githash.sh ${GIT_EXECUTABLE}
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}"
        RESULT_VARIABLE res
        OUTPUT_VARIABLE GIT_HASH
        ERROR_QUIET
        OUTPUT_STRIP_TRAILING_WHITESPACE)
    # This will cause reconfiguration on every commit
    set_property(DIRECTORY APPEND PROPERTY CMAKE_CONFIGURE_DEPENDS "${PROJECT_SOURCE_DIR}/.git/index")
else()
    set(GIT_HASH "no-git")
endif()
configure_file("${PROJECT_SOURCE_DIR}/cmake/version.go.in" "${PROJECT_SOURCE_DIR}/misc/version.go")

message(STATUS "Building version \"${PRJ_VERSION_MAJOR}.${PRJ_VERSION_MINOR}.${PRJ_VERSION_PATCH}\" git \"${GIT_HASH}\"")

if(CMAKE_BUILD_TYPE STREQUAL "Debug")
    # Create git pre-commit hook to check formatting before commit
    if(EXISTS "${PROJECT_SOURCE_DIR}/.git" AND IS_DIRECTORY "${PROJECT_SOURCE_DIR}/.git")
        execute_process(COMMAND ${CMAKE_COMMAND} -E copy ${PROJECT_SOURCE_DIR}/cmake/pre-commit ${PROJECT_SOURCE_DIR}/.git/hooks/. WORKING_DIRECTORY ${PROJECT_SOURCE_DIR})
    endif()
endif()

########################################################################################################
# Some contexts dependent settings
########################################################################################################

list(APPEND GO_ENV
    GOPATH=${GO_PATH}
    GOOS=${CROSS_GOOS}
    GOARCH=${CROSS_GOARCH}
    CGO_ENABLED=0
    )

if(CMAKE_BUILD_TYPE STREQUAL "Debug")
    set(TEST_RUN_ARGS "-v")
    set(GO_MOD_ARGS "-mod=mod")
else()
    set(GO_MOD_ARGS "-mod=vendor")
endif()

########################################################################################################
# main target - always out of date, "go build" will figure it out
########################################################################################################

add_custom_target(release
    DEPENDS ${PROJECT_SOURCE_DIR}/fb2c_${MSYSTEM_NAME}.zip
    COMMENT "Building release..."
    WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}")

set(FB2C_TARGET "${PROJECT_BINARY_DIR}${INSTALL_SUBDIR}/fb2c${CROSS_EXECUTABLE_SUFFIX}")
if(DEFINED KINDLEGEN)
    set(KINDLEGEN_TARGET "${PROJECT_BINARY_DIR}${INSTALL_SUBDIR}/kindlegen${CROSS_EXECUTABLE_SUFFIX}")
endif()
list(APPEND ZIP_DEPS ${FB2C_TARGET} ${KINDLEGEN_TARGET})
if(CROSS_GOOS STREQUAL "windows")
    # myhomelib suppport
    list(APPEND ZIP_DEPS ${PROJECT_BINARY_DIR}/fb2mobi/fb2mobi${CROSS_EXECUTABLE_SUFFIX} ${PROJECT_BINARY_DIR}/fb2epub/fb2epub${CROSS_EXECUTABLE_SUFFIX})
endif()
# strip path for archive content
foreach(fname ${ZIP_DEPS})
    file(RELATIVE_PATH path ${PROJECT_BINARY_DIR} ${fname})
    list(APPEND ZIP_CNTS ${path})
endforeach()

add_custom_command(OUTPUT ${PROJECT_SOURCE_DIR}/fb2c_${MSYSTEM_NAME}.zip
    DEPENDS ${ZIP_DEPS}
    COMMAND ${CMAKE_COMMAND} -E tar "cfv" ${PROJECT_SOURCE_DIR}/fb2c_${MSYSTEM_NAME}.zip --format=zip ${ZIP_CNTS}
    COMMENT "Archiving release..."
    WORKING_DIRECTORY "${PROJECT_BINARY_DIR}")

# make sure go always runs here - it will figure out the rest
if(CMAKE_BUILD_TYPE STREQUAL "Debug")
    add_custom_target(bin_fb2c ALL
        DEPENDS ${PROJECT_SOURCE_DIR}/processor/enums_string.go ${KINDLEGEN_TARGET}
        BYPRODUCTS ${FB2C_TARGET}
        COMMAND ${GO_ENV} ${GO_EXECUTABLE} build -gcflags 'all=-N -l' ${GO_MOD_ARGS} -o ${FB2C_TARGET}
            fb2converter/cmd/fb2c
        COMMENT "Building fb2c..."
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}")
else()
    add_custom_target(bin_fb2c ALL
        DEPENDS ${PROJECT_SOURCE_DIR}/processor/enums_string.go ${KINDLEGEN_TARGET}
        BYPRODUCTS ${FB2C_TARGET}
        COMMAND ${GO_ENV} ${GO_EXECUTABLE} build ${GO_MOD_ARGS} -trimpath -o ${FB2C_TARGET}
            fb2converter/cmd/fb2c
        COMMENT "Building fb2c..."
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}")
endif()

if(CROSS_GOOS STREQUAL "windows")
    # myhomelib suppport
    add_custom_command(OUTPUT ${PROJECT_BINARY_DIR}/fb2mobi/fb2mobi${CROSS_EXECUTABLE_SUFFIX}
        COMMAND ${GO_ENV} ${GO_EXECUTABLE} build ${GO_MOD_ARGS} -trimpath -o ${PROJECT_BINARY_DIR}/fb2mobi/fb2mobi${CROSS_EXECUTABLE_SUFFIX}
            fb2converter/cmd/fb2mobi
        COMMENT "Building fb2mobi..."
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}")
    add_custom_command(OUTPUT ${PROJECT_BINARY_DIR}/fb2epub/fb2epub${CROSS_EXECUTABLE_SUFFIX}
        COMMAND ${GO_ENV} ${GO_EXECUTABLE} build ${GO_MOD_ARGS} -trimpath -o ${PROJECT_BINARY_DIR}/fb2epub/fb2epub${CROSS_EXECUTABLE_SUFFIX}
             fb2converter/cmd/fb2epub
        COMMENT "Building fb2epub..."
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}")
endif()

if(DEFINED KINDLEGEN)
    add_custom_command(OUTPUT ${KINDLEGEN_TARGET}
        DEPENDS ${KINDLEGEN}
        COMMAND ${CMAKE_COMMAND} -E copy ${KINDLEGEN} ${KINDLEGEN_TARGET}
        COMMENT "Copying kindlegen..."
        WORKING_DIRECTORY "${PROJECT_BINARY_DIR}")
endif()

########################################################################################################
# Test targets: to test various parts of the system specify multiple test targets
########################################################################################################

# Temporary directory for test results
# execute_process(COMMAND ${CMAKE_COMMAND} -E make_directory tests_results)

add_custom_target(test_all
    DEPENDS test_processor test_hyphenator
    WORKING_DIRECTORY ${PROJECT_SOURCE_DIR}/src/dimple
    COMMENT "Running ALL tests...")

add_custom_target(test_processor
    DEPENDS ${PROJECT_SOURCE_DIR}/processor/enums_string.go
    COMMAND ${GO_ENV} ${GO_EXECUTABLE} test ${GO_MOD_ARGS} ${TEST_RUN_ARGS}
            -coverprofile=${PROJECT_BINARY_DIR}/test_processor.out
            fb2converter/processor
    COMMENT "processor tests..."
    WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}")

add_custom_target(test_hyphenator
    DEPENDS ${PROJECT_SOURCE_DIR}/processor/enums_string.go
    COMMAND ${GO_ENV} ${GO_EXECUTABLE} test ${TEST_RUN_ARGS}
            -coverprofile=${PROJECT_BINARY_DIR}/test_hyphenator.out
            ${GO_MOD_ARGS} fb2converter/hyphenator
    COMMENT "hyphenator tests..."
    WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}")

########################################################################################################
# Development
########################################################################################################

if(CMAKE_BUILD_TYPE STREQUAL "Debug")

    ########################################################################################################
    # Code generation - on host platform only
    ########################################################################################################

    add_custom_command(OUTPUT ${PROJECT_BINARY_DIR}/stringer
        COMMAND GOPATH=${GO_PATH} ${GO_EXECUTABLE} build ${GO_MOD_ARGS} -o ${PROJECT_BINARY_DIR}/stringer
            golang.org/x/tools/cmd/stringer
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}"
        COMMENT "Building stringer executable...")
    add_custom_target(bin_stringer DEPENDS ${PROJECT_BINARY_DIR}/stringer)

    add_custom_command(OUTPUT ${PROJECT_SOURCE_DIR}/processor/enums_string.go
        DEPENDS ${PROJECT_BINARY_DIR}/stringer
            ${PROJECT_SOURCE_DIR}/processor/enums.go
        COMMAND GOPATH=${GO_PATH} ${PROJECT_BINARY_DIR}/stringer
                -linecomment -type OutputFmt,NotesFmt,TOCPlacement,TOCType,APNXGeneration,StampPlacement,CoverProcessing
                -output processor/enums_string.go
                processor/enums.go
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}"
        COMMENT "Generating stringer interfaces")

    ########################################################################################################
    # Go linter
    ########################################################################################################

    add_custom_command(OUTPUT ${PROJECT_BINARY_DIR}/staticcheck
        COMMAND GOPATH=${GO_PATH} GOOS= GOARCH= ${GO_EXECUTABLE} build ${GO_MOD_ARGS} -o ${PROJECT_BINARY_DIR}/staticcheck
            honnef.co/go/tools/cmd/staticcheck
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}"
        COMMENT "Building staticcheck executable...")

    add_custom_target(lint
        DEPENDS ${PROJECT_BINARY_DIR}/staticcheck
        COMMAND ${PROJECT_BINARY_DIR}/staticcheck -f stylish -tests=false ./...
        COMMENT "Linting project..."
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}")

    ########################################################################################################
    # Go dependency management
    ########################################################################################################

    add_custom_target(deps_tidy
        SOURCES ${PROJECT_SOURCE_DIR}/go.mod
        COMMAND ${GO_ENV} ${GO_EXECUTABLE} mod tidy -go=${GO_LANG_VERSION}
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}"
        COMMENT "Cleaning dependencies (go mod tidy)"
        VERBATIM)

    add_custom_target(deps_download
        SOURCES ${PROJECT_SOURCE_DIR}/go.mod
        COMMAND ${GO_ENV} ${GO_EXECUTABLE} mod download all
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}"
        COMMENT "Downloading dependencies (go mod download)"
        VERBATIM)

    add_custom_target(deps_vendor
        SOURCES ${PROJECT_SOURCE_DIR}/go.mod
        COMMAND ${GO_ENV} ${GO_EXECUTABLE} mod vendor
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}"
        COMMENT "Creating vendor directory (go mod vendor)"
        VERBATIM)

endif()
